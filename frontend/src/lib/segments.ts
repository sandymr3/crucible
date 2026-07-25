import type { Verdict } from './verdict'

/**
 * A graded stretch of the candidate's answer, positioned in JavaScript string
 * indices.
 *
 * NOTE the units. The backend sends UTF-8 BYTE offsets; converting them is
 * lib/byteOffset's job, and by the time a range reaches here it must already be
 * in UTF-16 character indices.
 */
export interface SpanRange {
  start: number
  end: number
  verdict: Verdict
  concept: string
  explanation: string
  correction?: string
}

/** One piece of the rendered transcript: plain text, or a graded span. */
export type Segment =
  | { kind: 'text'; text: string }
  | { kind: 'span'; text: string; span: SpanRange; index: number }

/**
 * Splits a transcript into plain and graded pieces.
 *
 * Pure and total: it never throws on bad input, because the alternative is a
 * blank transcript during a live interview. Ranges that cannot be rendered
 * safely are dropped, following the backend's own rule for spans it cannot
 * anchor — a missing highlight is invisible, a misplaced one is a visible bug.
 *
 * Three defences, in order:
 *
 *  1. Out-of-bounds and inverted ranges are discarded rather than clamped.
 *     Clamping would move a verdict onto words that did not make the claim.
 *  2. Ranges are sorted by start, since arrival order is not position order.
 *  3. A range overlapping one already accepted is dropped. The backend anchors
 *     each excerpt independently and does not guarantee disjointness, and
 *     overlapping highlights cannot be represented as flat inline elements
 *     without nesting one verdict inside another.
 *
 * `index` counts accepted spans in document order, which is what drives the
 * left-to-right reveal stagger.
 */
export function segmentByRanges(text: string, ranges: SpanRange[]): Segment[] {
  if (!text) return []

  const usable = ranges
    .filter((r) => Number.isInteger(r.start) && Number.isInteger(r.end))
    .filter((r) => r.start >= 0 && r.end <= text.length && r.start < r.end)
    .sort((a, b) => a.start - b.start || a.end - b.end)

  const segments: Segment[] = []
  let cursor = 0
  let index = 0

  for (const range of usable) {
    if (range.start < cursor) continue // overlaps an accepted span

    if (range.start > cursor) {
      segments.push({ kind: 'text', text: text.slice(cursor, range.start) })
    }
    segments.push({
      kind: 'span',
      text: text.slice(range.start, range.end),
      span: range,
      index: index++,
    })
    cursor = range.end
  }

  if (cursor < text.length) {
    segments.push({ kind: 'text', text: text.slice(cursor) })
  }
  return segments
}
