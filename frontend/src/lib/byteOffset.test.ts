import { describe, expect, it } from 'vitest'

import {
  byteRangeToCharRange,
  byteToCharMap,
  resolveSpanRanges,
  sliceByBytes,
  utf8ByteLength,
  type ByteSpan,
} from './byteOffset'

/**
 * The highest-value test in the frontend.
 *
 * Expected byte offsets are produced with TextEncoder rather than hand-counted,
 * so the test cannot drift into agreeing with a bug in the implementation.
 */
const encoder = new TextEncoder()

/** What Go would report as the byte offset of `needle` inside `text`. */
function goByteOffsetOf(text: string, needle: string) {
  const at = text.indexOf(needle)
  if (at < 0) throw new Error(`test fixture: ${JSON.stringify(needle)} not in text`)
  const start = encoder.encode(text.slice(0, at)).length
  return { start, end: start + encoder.encode(needle).length }
}

/** Round-trips a substring through byte offsets, the way the backend does. */
function roundTrip(text: string, needle: string) {
  const { start, end } = goByteOffsetOf(text, needle)
  return sliceByBytes(text, byteToCharMap(text), start, end)
}

describe('utf8ByteLength', () => {
  it('agrees with TextEncoder across the encoding widths', () => {
    for (const sample of ['', 'ascii', 'naïve', 'a — b', '🔥', 'naïve — 🔥 done', '日本語']) {
      expect(utf8ByteLength(sample)).toBe(encoder.encode(sample).length)
    }
  })
})

describe('byteToCharMap', () => {
  it('is the identity for ASCII', () => {
    const text = 'a bloom filter'
    const map = byteToCharMap(text)
    for (let i = 0; i <= text.length; i++) expect(map[i]).toBe(i)
  })

  it('carries a one-past-the-end entry', () => {
    const text = 'naïve'
    const map = byteToCharMap(text)
    expect(map[utf8ByteLength(text)]).toBe(text.length)
  })

  it('maps every byte of a multi-byte character to that character', () => {
    // 'ï' is U+00EF: two bytes, one UTF-16 unit.
    const map = byteToCharMap('naïve')
    expect(map[0]).toBe(0) // n
    expect(map[1]).toBe(1) // a
    expect(map[2]).toBe(2) // ï, first byte
    expect(map[3]).toBe(2) // ï, second byte
    expect(map[4]).toBe(3) // v
  })

  it('counts an astral code point as two UTF-16 units', () => {
    // '🔥' is U+1F525: four bytes, two UTF-16 units.
    const text = 'a🔥b'
    const map = byteToCharMap(text)
    expect(map[0]).toBe(0) // a
    expect(map[1]).toBe(1) // 🔥, byte 1 -> high surrogate
    expect(map[4]).toBe(1) // 🔥, byte 4
    expect(map[5]).toBe(3) // b sits at UTF-16 index 3, not 2
    expect(map[6]).toBe(4) // one past the end
  })
})

describe('sliceByBytes', () => {
  it('recovers ASCII substrings', () => {
    const text = 'So the ingestion layer used a bloom filter downstream.'
    expect(roundTrip(text, 'a bloom filter')).toBe('a bloom filter')
  })

  // Each of these characters shifts every later byte offset. Before the
  // conversion existed, every highlight after the first one landed wrong.
  it.each([
    ['two-byte, an accent', 'That was a naïve approach to backpressure', 'backpressure'],
    ['three-byte, an em dash', 'We buffered it — that was the whole design', 'the whole design'],
    ['three-byte, a curly quote', 'It didn’t hold under load at all', 'under load'],
    ['four-byte, an emoji', 'Throughput 🔥 held at 2000 req/s', '2000 req/s'],
    ['several combined', 'naïve — it didn’t 🔥 hold, so we added flow control', 'flow control'],
  ])('recovers a substring after a %s', (_label, text, needle) => {
    expect(roundTrip(text, needle)).toBe(needle)
  })

  it('recovers the multi-byte characters themselves', () => {
    expect(roundTrip('a naïve design', 'naïve')).toBe('naïve')
    expect(roundTrip('held 🔥 steady', '🔥')).toBe('🔥')
  })

  it('recovers a span running to the very end of the string', () => {
    const text = 'it didn’t hold'
    expect(roundTrip(text, 'hold')).toBe('hold')
  })

  it('returns empty rather than guessing on an unresolvable range', () => {
    const text = 'naïve'
    const map = byteToCharMap(text)
    expect(sliceByBytes(text, map, 0, 999)).toBe('')
    expect(sliceByBytes(text, map, 3, 1)).toBe('')
  })
})

describe('byteRangeToCharRange', () => {
  const map = byteToCharMap('naïve')

  it('rejects ranges it cannot resolve rather than clamping them', () => {
    // Clamping would move a verdict onto words that never made the claim.
    expect(byteRangeToCharRange(map, -1, 3)).toBeNull()
    expect(byteRangeToCharRange(map, 2, 2)).toBeNull()
    expect(byteRangeToCharRange(map, 3, 2)).toBeNull()
    expect(byteRangeToCharRange(map, 0, 900)).toBeNull()
    expect(byteRangeToCharRange(map, 1.5, 3)).toBeNull()
    expect(byteRangeToCharRange(map, 0, Number.NaN)).toBeNull()
  })
})

describe('resolveSpanRanges', () => {
  const text = 'That was naïve — backpressure is not a bigger buffer, and we held 2000 req/s.'

  function span(excerpt: string, extra: Partial<ByteSpan> = {}): ByteSpan {
    const { start, end } = goByteOffsetOf(text, excerpt)
    return {
      start,
      end,
      excerpt,
      verdict: 'incorrect',
      concept: 'backpressure',
      explanation: 'A bigger buffer delays the problem.',
      ...extra,
    }
  }

  it('converts spans that sit after multi-byte characters', () => {
    const [resolved] = resolveSpanRanges(text, [span('backpressure is not a bigger buffer')])
    expect(text.slice(resolved.start, resolved.end)).toBe('backpressure is not a bigger buffer')
  })

  it('preserves the verdict, concept, explanation and correction', () => {
    const [resolved] = resolveSpanRanges(text, [
      span('2000 req/s', {
        verdict: 'unsupported',
        concept: 'throughput claim',
        correction: 'Measured how, at what percentile?',
      }),
    ])
    expect(resolved).toMatchObject({
      verdict: 'unsupported',
      concept: 'throughput claim',
      correction: 'Measured how, at what percentile?',
    })
  })

  it('omits correction when the backend did not send one', () => {
    const [resolved] = resolveSpanRanges(text, [span('naïve', { verdict: 'incomplete' })])
    expect('correction' in resolved).toBe(false)
  })

  it('drops a span whose verdict is not one of the four', () => {
    // An unrecognised verdict cannot be coloured, and guessing would invent a
    // judgement the model never made.
    expect(resolveSpanRanges(text, [span('naïve', { verdict: 'probably-fine' })])).toEqual([])
  })

  it('falls back to the excerpt when the offsets are wrong', () => {
    // Simulates protocol drift — offsets that do not describe their excerpt.
    const drifted = span('2000 req/s')
    drifted.start += 3
    drifted.end += 3

    const [resolved] = resolveSpanRanges(text, [drifted])
    expect(text.slice(resolved.start, resolved.end)).toBe('2000 req/s')
  })

  it('prefers the offsets over the excerpt when an excerpt repeats', () => {
    // indexOf would find the first "buffer"; the graded one is the second.
    const repeated = 'the buffer grew, then the buffer overflowed'
    const second = repeated.lastIndexOf('buffer')
    const s: ByteSpan = {
      start: encoder.encode(repeated.slice(0, second)).length,
      end: encoder.encode(repeated.slice(0, second + 'buffer'.length)).length,
      excerpt: 'buffer',
      verdict: 'incomplete',
      concept: 'buffering',
      explanation: 'Which one?',
    }

    const [resolved] = resolveSpanRanges(repeated, [s])
    expect(resolved.start).toBe(second)
  })

  it('drops a span whose excerpt is nowhere in the transcript', () => {
    // Built directly: the fixture helper deliberately refuses an excerpt that
    // is not present, and this case needs exactly that.
    const ghost: ByteSpan = {
      start: 0,
      end: 4,
      excerpt: 'a phrase that was never said',
      verdict: 'incorrect',
      concept: 'ghost',
      explanation: 'nothing to attach to',
    }
    expect(resolveSpanRanges(text, [ghost])).toEqual([])
  })

  it('returns nothing for an empty transcript or no spans', () => {
    expect(resolveSpanRanges('', [span('naïve')])).toEqual([])
    expect(resolveSpanRanges(text, [])).toEqual([])
  })

  it('resolves a full turn of spans against a realistic transcript', () => {
    const excerpts = ['naïve', 'backpressure is not a bigger buffer', '2000 req/s']
    const resolved = resolveSpanRanges(
      text,
      excerpts.map((e) => span(e)),
    )
    expect(resolved.map((r) => text.slice(r.start, r.end))).toEqual(excerpts)
  })
})
