import { create } from 'zustand'

import * as api from '../lib/api'
import { AudioPipeline, type PlaybackStats } from '../lib/audio'
import { setBand } from '../lib/band'
import { resolveSpanRanges, type ByteSpan } from '../lib/byteOffset'
import { getIdToken } from '../lib/firebase'
import type { PersonaId } from '../lib/persona'
import type { LiveState, ServerFrame } from '../lib/protocol'
import { BAND_ANNOUNCE_DELAY_MS } from '../lib/reveal'
import type { SpanRange } from '../lib/segments'
import { LiveSocket, type SocketStats } from '../lib/socket'
import type { Evaluation, Mode, Session } from '../lib/types'
import { pushToast } from './toasts'

/**
 * One question-and-answer exchange, as the client observes it.
 *
 * The relay never sends a turn object — the client assembles one from the
 * transcript deltas, the state transitions, and the evaluation that lands
 * several seconds later.
 */
export interface LiveTurn {
  index: number
  /** The interviewer's question, from the ai-side transcript deltas. */
  question: string
  /** The candidate's answer, from finalised user-side deltas. */
  answer: string
  /** Unfinalised user text. Render dimmer; treat as an enhancement only. */
  interim: string
  evaluation: Evaluation | null
  /** Character ranges, converted from the evaluation's byte offsets. */
  spans: SpanRange[]
  /** Set when a turn could not be graded, or was too short to be worth it. */
  ungraded: string | null
  hints: { text: string; penalty: number }[]
  band: number
  closed: boolean
  /** Wall clock when the turn opened, for the transcript's hanging timestamps. */
  at: number
}

export interface LiveError {
  code: string
  message: string
  recoverable: boolean
}

export function emptyTurn(index: number, band: number): LiveTurn {
  return {
    index,
    question: '',
    answer: '',
    interim: '',
    evaluation: null,
    spans: [],
    ungraded: null,
    hints: [],
    band,
    closed: false,
    at: Date.now(),
  }
}

interface SessionState {
  sessionId: string | null
  session: Session | null
  mode: Mode | null
  persona: PersonaId | null

  connection: 'idle' | 'connecting' | 'open' | 'closed'
  state: LiveState
  micHot: boolean

  turns: LiveTurn[]
  band: number
  /** Drives the flare sweep. `at` changes on every promotion or demotion. */
  lastBandChange: { from: number; to: number; at: number } | null
  /**
   * Every band the session has occupied, in order. Accumulated client-side:
   * the session document's bandHistory is written by the grader and is not
   * pushed back down the socket.
   */
  bandTrajectory: number[]
  hintsUsed: number

  usage: { totalTokens: number; audioIn: number; audioOut: number }
  socketStats: SocketStats | null
  playbackStats: PlaybackStats | null

  error: LiveError | null
  /**
   * The socket dropped and there is NO reconnect path on the backend —
   * resumption handles are emitted but nothing consumes them. Offer text mode
   * and "end and see my report"; never pretend to reconnect.
   */
  connectionLost: boolean

  startedAt: number | null
  ending: boolean

  // actions
  start: (sessionId: string) => Promise<void>
  beginInterview: () => void
  startAnswer: () => void
  endAnswer: () => void
  sendText: (text: string) => void
  requestHint: () => void
  end: () => Promise<void>
  /** Dismiss the connection notice. Only meaningful while the socket lives. */
  dismissConnectionLost: () => void
  reset: () => void
}

/**
 * Kept outside the store. They are long-lived mutable objects with their own
 * lifecycles; putting them in state would have every equality check walk an
 * AudioContext.
 */
let pipeline: AudioPipeline | null = null
let socket: LiveSocket | null = null

/** The backend's hard cap. There is no warning frame, so the client tracks it. */
export const SESSION_MAX_MS = 12 * 60 * 1000

/**
 * Every non-action field, at rest.
 *
 * Exported so tests build their harness from the real shape rather than
 * hand-listing fields — a hand-built copy silently drifts every time a field is
 * added, and the failure surfaces as an unrelated crash inside a reducer.
 */
export const initialSessionState = {
  sessionId: null,
  session: null,
  mode: null,
  persona: null,
  connection: 'idle',
  state: 'CONNECTING',
  micHot: false,
  turns: [],
  band: 3,
  lastBandChange: null,
  bandTrajectory: [],
  hintsUsed: 0,
  usage: { totalTokens: 0, audioIn: 0, audioOut: 0 },
  socketStats: null,
  playbackStats: null,
  error: null,
  connectionLost: false,
  startedAt: null,
  ending: false,
} satisfies Partial<SessionState>

export const useSession = create<SessionState>((set, get) => ({
  ...initialSessionState,

  async start(sessionId) {
    set({ ...initialSessionState, sessionId, connection: 'connecting' })

    try {
      const session = await api.getSession(sessionId)
      const band = session.difficultyBand || 3
      setBand(band)
      set({
        session,
        mode: session.mode,
        persona: session.persona ?? null,
        band,
        bandTrajectory: [band],
        turns: [emptyTurn(0, band)],
      })

      // Playback comes up BEFORE the socket. `begin` must not be sent until
      // the page can actually play: the opening question is the strongest
      // moment in the product and must never be spoken into a silent page.
      pipeline = new AudioPipeline()
      await pipeline.startPlayback()
      pipeline.onPlaybackStats((stats) => set({ playbackStats: stats }))

      // A replay session plays a recording and never needs the microphone, so
      // it must not prompt for one.
      if (session.mode !== 'replay') {
        await pipeline.startCapture()
        pipeline.onCaptureFrame(({ pcm }) => socket?.sendAudio(pcm))
      }

      const token = await getIdToken()
      if (!token) throw new Error('Not signed in.')

      socket = new LiveSocket(api.liveSocketUrl(sessionId, token), {
        onOpen: () => set({ connection: 'open', startedAt: Date.now() }),
        onFrame: (frame) => handleFrame(frame, set, get),
        onAudio: (pcm) => {
          pipeline?.play(pcm)
          if (socket) set({ socketStats: socket.getStats() })
        },
        onClose: () => {
          const { ending, state } = get()
          set({ connection: 'closed', micHot: false })
          // A close we did not ask for, before the session settled, is the
          // unrecoverable case.
          if (!ending && state !== 'SETTLED') set({ connectionLost: true })
        },
        onSocketError: () => set({ connectionLost: true }),
      })
      socket.connect()
    } catch (error) {
      set({
        connection: 'closed',
        state: 'ERROR',
        error: {
          code: error instanceof api.ApiError ? error.code : 'start_failed',
          message: error instanceof Error ? error.message : String(error),
          recoverable: false,
        },
      })
      await teardown()
    }
  },

  beginInterview() {
    socket?.signal('begin')
  },

  startAnswer() {
    if (!socket?.canSendAudio) return
    pipeline?.setMicHot(true)
    socket.signal('activity_start')
    set({ micHot: true })
  },

  endAnswer() {
    // The gate closes BEFORE the signal so no frame can be captured after the
    // boundary and land in the next turn.
    pipeline?.setMicHot(false)
    socket?.signal('activity_end')
    set({ micHot: false })
  },

  sendText(text) {
    const trimmed = text.trim()
    if (!trimmed) return
    // Typed answers travel the identical downstream path as speech, which is
    // what makes this a real fallback rather than a lesser mode.
    pipeline?.setMicHot(false)
    socket?.sendTextAnswer(trimmed)
    set({ micHot: false })
  },

  requestHint() {
    socket?.signal('request_hint')
  },

  async end() {
    const { sessionId } = get()
    set({ ending: true })
    socket?.close()
    await teardown()

    // THE ONLY thing that queues a report. Socket teardown alone leaves the
    // session in `evaluating` forever. Idempotent, so racing an unload handler
    // is safe.
    if (sessionId) {
      try {
        await api.endSession(sessionId)
      } catch {
        // The user is leaving; a failure here must not block them.
      }
    }
    set({ connection: 'closed' })
  },

  dismissConnectionLost() {
    set({ connectionLost: false })
  },

  reset() {
    void teardown()
    set({ ...initialSessionState })
  },
}))

/**
 * Whether typing an answer would actually reach the interviewer.
 *
 * text_answer travels the identical downstream path as speech — but only while
 * the socket is open. All three connection-ending error codes are followed by
 * the relay tearing down, so once it has closed there is nothing to type into.
 * Offering "continue in text mode" then would be exactly the dishonest
 * degradation the no-reconnect rule exists to prevent.
 */
export function canContinueInText(state: SessionState): boolean {
  return state.connection === 'open'
}

async function teardown() {
  socket?.close()
  socket = null
  await pipeline?.stop()
  pipeline = null
}

export type Setter = (partial: Partial<SessionState>) => void
export type Getter = () => SessionState

/**
 * The whole protocol reducer, exported so it can be driven directly by tests.
 *
 * Every omitempty trap in the wire format is handled in here, and each one
 * fails silently if it is got wrong — so they are worth testing without a
 * socket, a browser, or a backend.
 */
export function handleFrame(frame: ServerFrame, set: Setter, get: Getter) {
  switch (frame.type) {
    case 'state':
      handleState(frame, set, get)
      break

    case 'transcript':
      handleTranscript(frame, set, get)
      break

    case 'evaluation':
      handleEvaluation(frame, set, get)
      break

    case 'ungraded': {
      const turns = [...get().turns]
      const target = oldestPending(turns)
      if (target >= 0) {
        turns[target] = {
          ...turns[target],
          ungraded: frame.message ?? 'This turn was not graded.',
        }
        set({ turns })
      }
      break
    }

    case 'band':
      handleBand(frame, set, get)
      break

    case 'hint': {
      const turns = [...get().turns]
      const current = turns.length - 1
      turns[current] = {
        ...turns[current],
        // `penalty` is omitempty, so a zero penalty arrives as no key at all.
        hints: [...turns[current].hints, { text: frame.text ?? '', penalty: frame.penalty ?? 0 }],
      }
      set({ turns, hintsUsed: get().hintsUsed + 1 })
      break
    }

    case 'usage':
      set({
        usage: {
          // All three are omitempty: a zero count arrives as no key.
          totalTokens: frame.totalTokens ?? get().usage.totalTokens,
          audioIn: frame.audioTokensIn ?? get().usage.audioIn,
          audioOut: frame.audioTokensOut ?? get().usage.audioOut,
        },
      })
      break

    case 'interrupted':
      // Discard queued playback at once. Letting the buffer drain has the
      // interviewer talk over its own interruption for two seconds.
      pipeline?.flush()
      break

    case 'error':
      handleError(frame, set)
      break

    // turn_complete and pong need no state of their own.
  }
}

function handleState(frame: ServerFrame, set: Setter, get: Getter) {
  const next = frame.state
  if (!next) return
  set({ state: next })

  if (next === 'EVALUATING') {
    // The turn just closed server-side. `turnIndex` is omitempty, so the FIRST
    // turn carries no index at all — defaulting to 0 rather than treating it
    // as missing is what keeps turn numbering aligned.
    const closingIndex = frame.turnIndex ?? 0
    const turns = [...get().turns]
    const current = turns.length - 1
    if (current < 0) return

    const turn = turns[current]
    // Mirrors the relay's own guard: a turn with nothing in it is never
    // persisted. Without this a repeated EVALUATING closes the freshly opened
    // turn and leaves a blank row in the transcript.
    if (turn.closed || (!turn.answer && !turn.question)) return

    turns[current] = { ...turn, index: closingIndex, closed: true }
    turns.push(emptyTurn(closingIndex + 1, get().band))
    set({ turns, hintsUsed: 0 })
  }
}

function handleTranscript(frame: ServerFrame, set: Setter, get: Getter) {
  const text = frame.text ?? ''
  if (!text) return

  const turns = [...get().turns]
  const current = turns.length - 1
  if (current < 0) return

  // `final` is omitempty: an INTERIM update arrives with no `final` key, so
  // this must test for true rather than for false.
  const isFinal = frame.final === true

  if (frame.side === 'ai') {
    // Sourced from the output transcription stream, so the text on screen is
    // exactly what was said aloud.
    turns[current] = { ...turns[current], question: turns[current].question + text }
  } else if (isFinal) {
    turns[current] = {
      ...turns[current],
      // A DELTA, not a replacement. Replacing shows only the last word — the
      // most common integration bug against this protocol.
      answer: turns[current].answer + text,
      interim: '',
    }
  } else {
    turns[current] = { ...turns[current], interim: text }
  }

  set({ turns })
}

function handleEvaluation(frame: ServerFrame, set: Setter, get: Getter) {
  const evaluation = frame.payload as Evaluation | undefined
  if (!evaluation) return

  const turns = [...get().turns]
  const target = matchTurn(turns, evaluation)
  if (target < 0) return

  // Byte offsets to character indices. Without this every highlight after the
  // first non-ASCII character in the answer lands on the wrong words.
  const spans = resolveSpanRanges(
    turns[target].answer,
    (evaluation.spans ?? []) as unknown as ByteSpan[],
  )

  turns[target] = { ...turns[target], evaluation, spans, ungraded: null }
  set({ turns })
}

/**
 * Finds which turn an evaluation belongs to.
 *
 * The frame carries a Firestore turnId the client has never seen — nothing in
 * the live protocol maps an id to a turn the client assembled. So this matches
 * against the oldest turn still awaiting a grade.
 *
 * FIFO alone is not quite safe: a turn whose grading failed is re-queued at the
 * back of the worker pool, so a later turn's evaluation can arrive first. The
 * excerpts give a cheap cross-check — every span's excerpt is documented as the
 * transcript's own wording — so when the oldest candidate does not contain
 * them, the remaining pending turns are searched before giving up.
 */
function matchTurn(turns: LiveTurn[], evaluation: Evaluation): number {
  const pending = turns
    .map((turn, index) => ({ turn, index }))
    .filter(({ turn }) => turn.closed && !turn.evaluation)

  if (pending.length === 0) return -1

  const excerpt = evaluation.spans?.[0]?.excerpt
  if (!excerpt) return pending[0].index

  const match = pending.find(({ turn }) => turn.answer.includes(excerpt))
  return (match ?? pending[0]).index
}

function oldestPending(turns: LiveTurn[]): number {
  return turns.findIndex((turn) => turn.closed && !turn.evaluation && !turn.ungraded)
}

function handleBand(frame: ServerFrame, set: Setter, get: Getter) {
  const to = frame.to
  if (typeof to !== 'number') return

  const from = frame.from ?? get().band

  // t=0 — the room begins moving. One attribute write drives the 1800ms
  // thermal transition and the 900ms width axis at once.
  setBand(to)
  set({
    band: to,
    lastBandChange: { from, to, at: Date.now() },
    bandTrajectory: [...get().bandTrajectory, to],
  })

  // t=120 — the explanation follows the room rather than arriving with it,
  // which is what makes the change read as a consequence rather than a
  // coincidence. Adaptation the user cannot perceive is worthless, and
  // `message` is the backend's own copy, written to be read.
  setTimeout(() => {
    pushToast({
      title: frame.text ?? `Band ${to}`,
      message: frame.message ?? (to > from ? 'Difficulty raised.' : 'Easing off.'),
      accent: 'var(--heat-hot)',
    })
  }, BAND_ANNOUNCE_DELAY_MS)
}

function handleError(frame: ServerFrame, set: Setter) {
  const code = frame.code ?? 'error'
  // `recoverable` is omitempty: absent means false.
  const recoverable = frame.recoverable === true

  set({
    error: {
      code,
      message: frame.message ?? 'Something went wrong.',
      recoverable,
    },
  })

  // These end the session for real. There is no reconnect path, so the UI must
  // degrade honestly rather than showing a spinner that will never resolve.
  if (code === 'live_stream_lost' || code === 'live_going_away' || code === 'idle_timeout') {
    set({ connectionLost: true, micHot: false })
  }
  if (code === 'live_connect_failed' || code === 'fixture_not_found') {
    set({ state: 'ERROR', connectionLost: true })
  }
}
