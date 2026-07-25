/**
 * The live session's vocabulary, as the relay emits it.
 *
 * The most common failure in a voice UI is that the user cannot tell whether
 * the system is listening, thinking, or dead — so the backend surfaces every
 * one of these explicitly rather than leaving the client to infer it, and every
 * state must have a distinct visual signature.
 */

export type LiveState =
  | 'CONNECTING'
  | 'ASKING'
  | 'LISTENING'
  | 'CLOSING'
  | 'EVALUATING'
  | 'SETTLED'
  | 'ERROR'

/**
 * Two of these never arrive over the wire, and it matters for anything that
 * switches exhaustively on state:
 *
 *   ERROR    is declared by the backend but never emitted. Failures arrive as a
 *            separate `error` frame and the state does not change, so the
 *            client sets this itself.
 *   SETTLED  is emitted only at the end of a REPLAY session. A live interview
 *            never sends it.
 */
export const CLIENT_ONLY_STATES: LiveState[] = ['ERROR']

/** The microphone is hot and the client may send audio. */
export function isListening(state: LiveState): boolean {
  return state === 'LISTENING'
}

/** The interviewer is speaking, or about to. */
export function isSpeaking(state: LiveState): boolean {
  return state === 'ASKING'
}

// ── Client → Server ───────────────────────────────────────────────────────

export type ClientFrameType =
  | 'begin'
  | 'activity_start'
  | 'activity_end'
  | 'text_answer'
  | 'request_hint'
  | 'end_session'
  | 'ping'

export interface ClientFrame {
  type: ClientFrameType
  text?: string
  t?: number
}

// ── Server → Client ───────────────────────────────────────────────────────

export type ServerFrameType =
  | 'state'
  | 'transcript'
  | 'turn_complete'
  | 'interrupted'
  | 'usage'
  | 'evaluation'
  | 'ungraded'
  | 'band'
  | 'hint'
  | 'error'
  | 'pong'

/**
 * One flat shape with everything optional, because that is literally what the
 * relay sends: a single Go struct with omitempty on almost every field.
 *
 * ⚠️ omitempty means a ZERO-VALUED FIELD IS ABSENT, not false or 0. In
 * TypeScript it arrives as `undefined`. The consequences that bite:
 *
 *   final        absent when false — an INTERIM transcript has no `final` key,
 *                so test `=== true`, never `=== false`.
 *   turnIndex    absent when 0 — the FIRST turn's EVALUATING frame carries no
 *                index. Default to 0 rather than treating it as missing.
 *   recoverable  absent when false — an unrecoverable error has no key.
 *   penalty      absent when 0.
 *   token counts absent when 0, so a usage frame can be nearly empty.
 */
export interface ServerFrame {
  type: ServerFrameType | string

  // transcript
  side?: 'user' | 'ai'
  text?: string
  final?: boolean

  // state
  state?: LiveState
  turnIndex?: number

  // error
  code?: string
  recoverable?: boolean
  message?: string

  // usage
  totalTokens?: number
  audioTokensIn?: number
  audioTokensOut?: number

  // evaluation / ungraded
  turnId?: string
  payload?: unknown

  // band
  from?: number
  to?: number

  // hint
  penalty?: number

  // pong
  t?: number
}

/**
 * Error codes the relay emits.
 *
 * There is NO reconnect path on the backend: resumption handles are emitted but
 * nothing consumes them, so a dropped socket ends the session. The client must
 * degrade honestly rather than pretending to reconnect — offer text mode, which
 * genuinely works because text_answer travels the identical downstream path.
 */
export type ErrorCode =
  | 'live_connect_failed'
  | 'live_stream_lost'
  | 'live_going_away'
  | 'idle_timeout'
  | 'hint_limit'
  | 'hint_failed'
  | 'fixture_not_found'

/** 4-byte big-endian sequence number prefixed to every downstream audio frame. */
export const AUDIO_SEQ_PREFIX_LEN = 4

/**
 * Splits a downstream binary frame into its sequence number and PCM payload.
 *
 * The sequence number exists so gaps and reordering are detectable, and so
 * buffer underruns are measurable rather than merely audible. Gap count is the
 * earliest warning of a network problem — well before anyone can hear it.
 */
export function decodeAudioFrame(buffer: ArrayBuffer): { seq: number; pcm: Int16Array } | null {
  if (buffer.byteLength < AUDIO_SEQ_PREFIX_LEN) return null
  const seq = new DataView(buffer).getUint32(0, false) // big-endian
  return { seq, pcm: new Int16Array(buffer.slice(AUDIO_SEQ_PREFIX_LEN)) }
}
