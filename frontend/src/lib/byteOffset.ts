import type { SpanRange } from './segments'
import { asVerdict } from './verdict'

/**
 * UTF-8 byte offsets → UTF-16 character indices.
 *
 * The backend is Go. `span.start` and `span.end` index a UTF-8 BYTE array;
 * JavaScript strings are UTF-16 code units. For pure ASCII the two coincide,
 * which is exactly why this bug ships: everything looks correct until a
 * transcript contains an accented character, an em dash, a curly quote, or an
 * emoji — and from that character onward EVERY highlight is misplaced.
 *
 * Speech transcription produces those constantly, so this is not an edge case.
 */

/** UTF-8 encoded length of a code point. Arithmetic, no allocation. */
function utf8Length(codePoint: number): number {
  if (codePoint < 0x80) return 1
  if (codePoint < 0x800) return 2
  if (codePoint < 0x10000) return 3
  return 4
}

/**
 * Builds a lookup from byte offset to character index.
 *
 * `map[byteOffset]` is the UTF-16 index of the character that byte belongs to,
 * and `map[byteLength]` is `text.length` — the one-past-the-end entry, so an
 * exclusive end offset at the very end of the string resolves.
 *
 * Build this ONCE per transcript, not once per span. A turn can carry twelve
 * spans and rebuilding the map for each is twelve full passes over the text.
 */
export function byteToCharMap(text: string): number[] {
  const map: number[] = []
  let byte = 0

  for (let ch = 0; ch < text.length; ) {
    const cp = text.codePointAt(ch)
    if (cp === undefined) break

    const width = utf8Length(cp)
    for (let b = 0; b < width; b++) map[byte + b] = ch
    byte += width

    // A code point above the BMP occupies two UTF-16 units. Advancing by one
    // would land on the low surrogate and count the character twice.
    ch += cp > 0xffff ? 2 : 1
  }

  map[byte] = text.length
  return map
}

/** Total UTF-8 byte length, without building a map. */
export function utf8ByteLength(text: string): number {
  let bytes = 0
  for (let ch = 0; ch < text.length; ) {
    const cp = text.codePointAt(ch)
    if (cp === undefined) break
    bytes += utf8Length(cp)
    ch += cp > 0xffff ? 2 : 1
  }
  return bytes
}

/**
 * Converts one byte range. Returns null when it cannot be resolved.
 *
 * Null rather than a clamped guess, following the backend's own rule for spans
 * it cannot anchor: a missing highlight is invisible, a misplaced one attaches
 * a verdict to words that never made the claim.
 */
export function byteRangeToCharRange(
  map: number[],
  start: number,
  end: number,
): { start: number; end: number } | null {
  if (!Number.isInteger(start) || !Number.isInteger(end)) return null
  if (start < 0 || end <= start) return null
  if (start >= map.length || end >= map.length) return null

  const charStart = map[start]
  const charEnd = map[end]
  if (charStart === undefined || charEnd === undefined) return null
  if (charEnd <= charStart) return null

  return { start: charStart, end: charEnd }
}

/** Slices a string by UTF-8 byte offsets. */
export function sliceByBytes(text: string, map: number[], start: number, end: number): string {
  const range = byteRangeToCharRange(map, start, end)
  return range ? text.slice(range.start, range.end) : ''
}

/** A span as the backend sends it: byte offsets, and its own verbatim excerpt. */
export interface ByteSpan {
  start: number
  end: number
  excerpt: string
  verdict: string
  concept: string
  explanation: string
  correction?: string
}

/**
 * Resolves backend spans into character ranges ready for segmentByRanges.
 *
 * The conversion is verified rather than trusted. `excerpt` is documented as
 * the transcript's own wording at [start,end), so slicing the converted range
 * must reproduce it exactly — which turns a silent off-by-N into something we
 * can detect and repair.
 *
 * Three outcomes per span:
 *
 *  1. The slice matches the excerpt. Use the converted range.
 *  2. It does not. Fall back to locating the excerpt directly. Tried SECOND and
 *     never first: an excerpt appearing more than once in an answer would have
 *     indexOf pick the earliest occurrence, which may not be the one graded.
 *  3. Neither works. Drop the span.
 *
 * Unrecognised verdicts are dropped too — an unknown verdict cannot be
 * coloured, and guessing would invent a judgement the model did not make.
 */
export function resolveSpanRanges(text: string, spans: ByteSpan[]): SpanRange[] {
  if (!text || spans.length === 0) return []

  const map = byteToCharMap(text)
  const resolved: SpanRange[] = []

  for (const span of spans) {
    const verdict = asVerdict(span.verdict)
    if (!verdict) continue

    const base = {
      verdict,
      concept: span.concept,
      explanation: span.explanation,
      ...(span.correction ? { correction: span.correction } : {}),
    }

    const converted = byteRangeToCharRange(map, span.start, span.end)
    if (converted && text.slice(converted.start, converted.end) === span.excerpt) {
      resolved.push({ ...base, ...converted })
      continue
    }

    const found = span.excerpt ? text.indexOf(span.excerpt) : -1
    if (found >= 0) {
      resolved.push({ ...base, start: found, end: found + span.excerpt.length })
    }
  }

  return resolved
}
