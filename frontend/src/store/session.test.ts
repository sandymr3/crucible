import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ServerFrame } from '../lib/protocol'
import { BAND_ANNOUNCE_DELAY_MS } from '../lib/reveal'
import type { Evaluation, Span } from '../lib/types'
import { emptyTurn, handleFrame, initialSessionState, type LiveTurn } from './session'
import { TOAST_TTL_MS, useToasts } from './toasts'

/**
 * Drives the protocol reducer directly — no socket, no backend, no browser.
 *
 * Most of what is asserted here is the omitempty behaviour of the wire format:
 * a zero-valued Go field is ABSENT, not false or 0. Every one of those fails
 * silently if handled wrongly.
 */
function harness(turns: LiveTurn[] = [emptyTurn(0, 3)]) {
  // Built from the store's own initial state rather than hand-listed, so
  // adding a field cannot silently leave the harness behind — which surfaces
  // as an unrelated crash inside whichever reducer touches it first.
  let state = {
    ...initialSessionState,
    turns,
  } as unknown as ReturnType<Parameters<typeof handleFrame>[2]>

  const set = (partial: object) => {
    state = { ...state, ...partial }
  }
  const get = () => state

  return {
    send: (frame: ServerFrame) => handleFrame(frame, set, get),
    get current() {
      return state
    },
  }
}

/** Byte offsets, the way Go computes them. */
function span(answer: string, excerpt: string, extra: Partial<Span> = {}): Span {
  const encoder = new TextEncoder()
  const at = answer.indexOf(excerpt)
  const start = encoder.encode(answer.slice(0, at)).length
  return {
    excerpt,
    verdict: 'incorrect',
    concept: 'backpressure',
    explanation: 'A bigger buffer delays the problem.',
    confidence: 0.9,
    start,
    end: start + encoder.encode(excerpt).length,
    ...extra,
  }
}

function evaluation(spans: Span[], overrides: Partial<Evaluation> = {}): Evaluation {
  return {
    turn_id: 't-abc',
    question_intent: 'probe backpressure',
    scores: { technical_accuracy: 5, communication_clarity: 6, depth: 4, structure: 5 },
    verdict_summary: 'Mostly right.',
    spans,
    concepts_demonstrated: [],
    concepts_missing: [],
    ideal_answer_outline: [],
    followup_probe: 'What signals the producer?',
    difficulty_recommendation: 'hold',
    turnScore: 5.2,
    spansDropped: 0,
    redsDowngraded: 0,
    gradedAt: '2026-07-26T00:00:00Z',
    durationMs: 4200,
    ...overrides,
  }
}

beforeEach(() => {
  useToasts.getState().clear()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('transcripts', () => {
  it('accumulates the ai side into the question', () => {
    const h = harness()
    h.send({ type: 'transcript', side: 'ai', text: 'Walk me through ', final: true })
    h.send({ type: 'transcript', side: 'ai', text: 'backpressure.', final: true })
    expect(h.current.turns[0].question).toBe('Walk me through backpressure.')
  })

  it('APPENDS user text rather than replacing it', () => {
    // Replacing shows only the last word — the most common integration bug
    // against this protocol.
    const h = harness()
    h.send({ type: 'transcript', side: 'user', text: 'We used ', final: true })
    h.send({ type: 'transcript', side: 'user', text: 'a bounded queue.', final: true })
    expect(h.current.turns[0].answer).toBe('We used a bounded queue.')
  })

  it('treats a frame with NO final key as interim', () => {
    // `final` is omitempty, so an interim update carries no key at all. Testing
    // for `final === false` would classify every interim frame as final.
    const h = harness()
    h.send({ type: 'transcript', side: 'user', text: 'we used a boun' })
    expect(h.current.turns[0].answer).toBe('')
    expect(h.current.turns[0].interim).toBe('we used a boun')
  })

  it('clears the interim text once the final lands', () => {
    const h = harness()
    h.send({ type: 'transcript', side: 'user', text: 'we used a boun' })
    h.send({ type: 'transcript', side: 'user', text: 'We used a bounded queue.', final: true })
    expect(h.current.turns[0]).toMatchObject({
      answer: 'We used a bounded queue.',
      interim: '',
    })
  })

  it('ignores an empty transcript frame', () => {
    const h = harness()
    h.send({ type: 'transcript', side: 'ai', text: '', final: true })
    expect(h.current.turns).toHaveLength(1)
    expect(h.current.turns[0].question).toBe('')
  })
})

describe('turn boundaries', () => {
  /** A turn that has actually been asked and answered, as one always has by
   *  the time the relay closes it. */
  function answered(h: ReturnType<typeof harness>) {
    h.send({ type: 'transcript', side: 'ai', text: 'Walk me through it.', final: true })
    h.send({ type: 'transcript', side: 'user', text: 'We used a bounded queue.', final: true })
  }

  it('closes turn 0 when EVALUATING arrives with NO turnIndex', () => {
    // turnIndex is omitempty, so the FIRST turn's frame carries no index.
    // Treating it as missing rather than as 0 misaligns every later turn.
    const h = harness()
    answered(h)
    h.send({ type: 'state', state: 'EVALUATING' })

    expect(h.current.turns[0]).toMatchObject({ index: 0, closed: true })
    expect(h.current.turns[1]).toMatchObject({ index: 1, closed: false })
  })

  it('uses the supplied index for later turns', () => {
    const h = harness()
    answered(h)
    h.send({ type: 'state', state: 'EVALUATING', turnIndex: 3 })
    expect(h.current.turns[0].index).toBe(3)
    expect(h.current.turns[1].index).toBe(4)
  })

  it('refuses to close a turn with nothing in it', () => {
    // Mirrors the relay, which never persists an empty turn. Without this a
    // repeated EVALUATING closes the freshly opened turn and leaves a blank
    // row in the transcript.
    const h = harness()
    answered(h)
    h.send({ type: 'state', state: 'EVALUATING' })
    h.send({ type: 'state', state: 'EVALUATING' })
    expect(h.current.turns).toHaveLength(2)
  })

  it('closes a turn that was asked but produced no transcript', () => {
    // Speech that fails to transcribe still has audio, so the relay closes the
    // turn and the client must follow — the question alone is content enough.
    const h = harness()
    h.send({ type: 'transcript', side: 'ai', text: 'Walk me through it.', final: true })
    h.send({ type: 'state', state: 'EVALUATING' })
    expect(h.current.turns[0].closed).toBe(true)
  })

  it('routes the next question into the new turn', () => {
    const h = harness()
    h.send({ type: 'transcript', side: 'ai', text: 'First question.', final: true })
    h.send({ type: 'transcript', side: 'user', text: 'An answer.', final: true })
    h.send({ type: 'state', state: 'EVALUATING' })
    h.send({ type: 'transcript', side: 'ai', text: 'Second question.', final: true })

    expect(h.current.turns[0].question).toBe('First question.')
    expect(h.current.turns[1].question).toBe('Second question.')
  })

  it('resets the per-turn hint count at the boundary', () => {
    const h = harness()
    answered(h)
    h.send({ type: 'hint', text: 'What signals the producer?', penalty: 0.5 })
    expect(h.current.hintsUsed).toBe(1)
    h.send({ type: 'state', state: 'EVALUATING' })
    expect(h.current.hintsUsed).toBe(0)
  })
})

describe('evaluations', () => {
  const answer = 'That was naïve — backpressure was just a bigger buffer, at 2000 req/s.'

  function closedTurn(index: number, text: string): LiveTurn {
    return { ...emptyTurn(index, 3), answer: text, closed: true }
  }

  it('attaches to the pending turn and converts byte offsets', () => {
    const h = harness([closedTurn(0, answer), emptyTurn(1, 3)])
    h.send({
      type: 'evaluation',
      turnId: 't-abc',
      payload: evaluation([span(answer, 'backpressure was just a bigger buffer')]),
    })

    const [resolved] = h.current.turns[0].spans
    // The offsets came from Go as UTF-8 bytes; the accent shifts them by one.
    expect(answer.slice(resolved.start, resolved.end)).toBe(
      'backpressure was just a bigger buffer',
    )
  })

  it('matches by excerpt when evaluations arrive out of order', () => {
    // A turn whose grading failed is re-queued at the back of the pool, so a
    // later turn's evaluation can land first. Plain FIFO would mis-attach it.
    const first = 'We used a bloom filter for dedup.'
    const second = 'Backpressure was just a bigger buffer.'
    const h = harness([closedTurn(0, first), closedTurn(1, second), emptyTurn(2, 3)])

    h.send({
      type: 'evaluation',
      payload: evaluation([span(second, 'a bigger buffer')]),
    })

    expect(h.current.turns[1].evaluation).not.toBeNull()
    expect(h.current.turns[0].evaluation).toBeNull()
  })

  it('falls back to the oldest pending turn when there are no spans', () => {
    const h = harness([closedTurn(0, answer), emptyTurn(1, 3)])
    h.send({ type: 'evaluation', payload: evaluation([]) })
    expect(h.current.turns[0].evaluation).not.toBeNull()
  })

  it('ignores an evaluation with no payload', () => {
    const h = harness([closedTurn(0, answer)])
    h.send({ type: 'evaluation', turnId: 't-abc' })
    expect(h.current.turns[0].evaluation).toBeNull()
  })

  it('never attaches to a turn that is still open', () => {
    const h = harness([emptyTurn(0, 3)])
    h.send({ type: 'evaluation', payload: evaluation([]) })
    expect(h.current.turns[0].evaluation).toBeNull()
  })
})

describe('ungraded turns', () => {
  it('marks the oldest pending turn', () => {
    const h = harness([{ ...emptyTurn(0, 3), answer: 'yes', closed: true }, emptyTurn(1, 3)])
    h.send({ type: 'ungraded', turnId: 't-1', message: 'Too short to grade.' })
    expect(h.current.turns[0].ungraded).toBe('Too short to grade.')
  })

  it('supplies copy when the backend sends none', () => {
    const h = harness([{ ...emptyTurn(0, 3), closed: true }])
    h.send({ type: 'ungraded', turnId: 't-1' })
    expect(h.current.turns[0].ungraded).toBeTruthy()
  })
})

describe('band changes', () => {
  it('moves the room at t=0, before anything is announced', () => {
    vi.useFakeTimers()
    const h = harness()
    h.send({ type: 'band', from: 3, to: 4, message: 'Difficulty raised.' })

    expect(h.current.band).toBe(4)
    // The signature: one attribute write drives the 1800ms thermal shift and
    // the 900ms width axis together.
    expect(document.documentElement.dataset.band).toBe('4')
    // The explanation must NOT arrive with the room. Landing together reads as
    // coincidence; landing after reads as consequence.
    expect(useToasts.getState().toasts).toHaveLength(0)
    vi.useRealTimers()
  })

  it('announces at t=120, carrying the backend copy verbatim', () => {
    vi.useFakeTimers()
    const h = harness()
    h.send({
      type: 'band',
      from: 3,
      to: 4,
      message: "Difficulty raised — you've proven the fundamentals.",
      text: 'Band 4 — Tradeoff',
    })

    vi.advanceTimersByTime(119)
    expect(useToasts.getState().toasts).toHaveLength(0)

    vi.advanceTimersByTime(1)
    expect(useToasts.getState().toasts[0]).toMatchObject({
      title: 'Band 4 — Tradeoff',
      message: "Difficulty raised — you've proven the fundamentals.",
    })
    vi.useRealTimers()
  })

  it('records the change so the flare can fire on every move', () => {
    // The band number alone is not enough: a demotion back to a band already
    // seen still has to announce itself.
    const h = harness()
    h.send({ type: 'band', from: 3, to: 4 })
    const first = h.current.lastBandChange
    expect(first).toMatchObject({ from: 3, to: 4 })

    h.send({ type: 'band', from: 4, to: 3 })
    expect(h.current.lastBandChange).toMatchObject({ from: 4, to: 3 })
    expect(h.current.lastBandChange?.at).toBeGreaterThanOrEqual(first!.at)
  })

  it('supplies copy when the backend sends none', () => {
    vi.useFakeTimers()
    const h = harness()
    h.send({ type: 'band', from: 4, to: 3 })
    vi.advanceTimersByTime(200)
    expect(useToasts.getState().toasts[0].message).toBe('Easing off.')
    vi.useRealTimers()
  })

  it('ignores a band frame with no target', () => {
    vi.useFakeTimers()
    const h = harness()
    h.send({ type: 'band', from: 3 })
    vi.advanceTimersByTime(500)
    expect(h.current.band).toBe(3)
    expect(h.current.lastBandChange).toBeNull()
    expect(useToasts.getState().toasts).toHaveLength(0)
    vi.useRealTimers()
  })

  it('dismisses the toast at t=4200', () => {
    vi.useFakeTimers()
    const h = harness()
    h.send({ type: 'band', from: 3, to: 5 })
    vi.advanceTimersByTime(BAND_ANNOUNCE_DELAY_MS)
    expect(useToasts.getState().toasts).toHaveLength(1)

    vi.advanceTimersByTime(TOAST_TTL_MS)
    expect(useToasts.getState().toasts).toHaveLength(0)
    vi.useRealTimers()
  })
})

describe('hints and usage', () => {
  it('records a hint with no penalty key as zero', () => {
    const h = harness()
    h.send({ type: 'hint', text: 'Think about the producer.' })
    expect(h.current.turns[0].hints[0]).toEqual({
      text: 'Think about the producer.',
      penalty: 0,
    })
  })

  it('keeps previous usage counts when a field is absent', () => {
    // Token counts are omitempty, so a usage frame can be nearly empty.
    const h = harness()
    h.send({ type: 'usage', totalTokens: 1200, audioTokensIn: 300, audioTokensOut: 400 })
    h.send({ type: 'usage', totalTokens: 1500 })
    expect(h.current.usage).toEqual({ totalTokens: 1500, audioIn: 300, audioOut: 400 })
  })
})

describe('errors', () => {
  it('reads a missing recoverable key as false', () => {
    const h = harness()
    h.send({ type: 'error', code: 'live_connect_failed', message: 'Could not reach.' })
    expect(h.current.error).toMatchObject({ code: 'live_connect_failed', recoverable: false })
  })

  it('marks the connection lost, since there is no reconnect path', () => {
    // Resumption handles are emitted but nothing consumes them. The UI must
    // offer text mode and "end and see my report", never a fake reconnect.
    for (const code of ['live_stream_lost', 'live_going_away', 'idle_timeout']) {
      const h = harness()
      h.send({ type: 'error', code, recoverable: true })
      expect(h.current.connectionLost).toBe(true)
      expect(h.current.micHot).toBe(false)
    }
  })

  it('does not mark the connection lost for a per-turn failure', () => {
    const h = harness()
    h.send({ type: 'error', code: 'hint_limit', recoverable: true })
    expect(h.current.connectionLost).toBe(false)
  })
})

describe('state transitions', () => {
  it('records every state the relay reports', () => {
    const h = harness()
    for (const state of ['CONNECTING', 'ASKING', 'LISTENING', 'CLOSING'] as const) {
      h.send({ type: 'state', state })
      expect(h.current.state).toBe(state)
    }
  })

  it('ignores a state frame with no state', () => {
    const h = harness()
    h.send({ type: 'state' })
    expect(h.current.state).toBe('CONNECTING')
  })
})
