import { describe, expect, it } from 'vitest'

import { segmentByRanges, type SpanRange } from './segments'

function range(start: number, end: number, extra: Partial<SpanRange> = {}): SpanRange {
  return {
    start,
    end,
    verdict: 'validated',
    concept: 'test',
    explanation: 'because',
    ...extra,
  }
}

describe('segmentByRanges', () => {
  it('returns nothing for empty text', () => {
    expect(segmentByRanges('', [range(0, 2)])).toEqual([])
  })

  it('returns one text segment when there are no ranges', () => {
    expect(segmentByRanges('hello', [])).toEqual([{ kind: 'text', text: 'hello' }])
  })

  it('splits text around a span', () => {
    const out = segmentByRanges('a bloom filter here', [range(2, 14)])
    expect(out.map((s) => s.text)).toEqual(['a ', 'bloom filter', ' here'])
    expect(out[1]).toMatchObject({ kind: 'span', index: 0 })
  })

  it('handles a span at the very start and the very end', () => {
    expect(segmentByRanges('abc', [range(0, 3)])).toEqual([
      { kind: 'span', text: 'abc', span: range(0, 3), index: 0 },
    ])
  })

  it('numbers spans in document order regardless of arrival order', () => {
    const out = segmentByRanges('one two three', [range(8, 13), range(0, 3)])
    const spans = out.filter((s) => s.kind === 'span')
    expect(spans.map((s) => [s.text, s.index])).toEqual([
      ['one', 0],
      ['three', 1],
    ])
  })

  it('drops a range that overlaps one already accepted', () => {
    // The backend anchors each excerpt independently and does not guarantee
    // disjointness. Overlaps cannot be rendered as flat inline elements.
    const out = segmentByRanges('one two three', [range(0, 7), range(4, 13)])
    expect(out.map((s) => s.text)).toEqual(['one two', ' three'])
  })

  it('keeps two ranges that merely touch', () => {
    const out = segmentByRanges('onetwo', [range(0, 3), range(3, 6)])
    expect(out.filter((s) => s.kind === 'span').map((s) => s.text)).toEqual(['one', 'two'])
  })

  it('drops out-of-bounds ranges rather than clamping them', () => {
    // Clamping would move a verdict onto words that never made the claim.
    expect(segmentByRanges('abc', [range(0, 99)])).toEqual([{ kind: 'text', text: 'abc' }])
    expect(segmentByRanges('abc', [range(-1, 2)])).toEqual([{ kind: 'text', text: 'abc' }])
  })

  it('drops inverted and empty ranges', () => {
    expect(segmentByRanges('abc', [range(2, 1)])).toEqual([{ kind: 'text', text: 'abc' }])
    expect(segmentByRanges('abc', [range(1, 1)])).toEqual([{ kind: 'text', text: 'abc' }])
  })

  it('drops non-integer ranges', () => {
    expect(segmentByRanges('abc', [range(0.5, 2)])).toEqual([{ kind: 'text', text: 'abc' }])
    expect(segmentByRanges('abc', [range(0, Number.NaN)])).toEqual([{ kind: 'text', text: 'abc' }])
  })

  it('reassembles to exactly the original transcript', () => {
    const text = 'So the ingestion layer used a Kafka topic per source, and we deduplicated.'
    const out = segmentByRanges(text, [range(7, 22), range(30, 41), range(61, 73)])
    expect(out.map((s) => s.text).join('')).toBe(text)
  })
})
