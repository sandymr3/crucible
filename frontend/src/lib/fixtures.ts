import type { ByteSpan } from './byteOffset'
import type { SpanRange } from './segments'
import type { Verdict } from './verdict'

/**
 * Hand-written demo content.
 *
 * The home page's hero performs the grading rather than describing it, and it
 * must render identically every time and never wait on a network request — so
 * this is hard-coded and the hero never calls the backend.
 *
 * Ranges are declared by excerpt and located with indexOf rather than written
 * as literal offsets. Hand-counted offsets in a fixture drift the moment
 * someone edits a word, and the failure is silent: the highlight simply lands
 * on the wrong phrase. This throws instead.
 */
function rangeOf(
  text: string,
  excerpt: string,
  span: Omit<SpanRange, 'start' | 'end'>,
): SpanRange {
  const start = text.indexOf(excerpt)
  if (start < 0) {
    throw new Error(`fixtures: excerpt not found in demo transcript: ${JSON.stringify(excerpt)}`)
  }
  return { ...span, start, end: start + excerpt.length }
}

export const DEMO_TRANSCRIPT =
  'So the ingestion layer used a Kafka topic per source, and we deduplicated ' +
  'downstream using a bloom filter before the feature store write. ' +
  'Backpressure was just a bigger buffer, and we handled 2000 req/s.'

export const DEMO_SPANS: SpanRange[] = [
  rangeOf(DEMO_TRANSCRIPT, 'a bloom filter', {
    verdict: 'validated',
    concept: 'deduplication',
    explanation:
      'Correct, and the right tool: a probabilistic set membership test is exactly what a dedup path wants when false positives are cheap.',
  }),
  rangeOf(DEMO_TRANSCRIPT, 'Backpressure was just a bigger buffer', {
    verdict: 'incorrect',
    concept: 'backpressure',
    explanation: 'A bigger buffer delays the problem. It is not flow control.',
    correction:
      'Backpressure signals the producer to slow down. A buffer only changes how long you have before the same failure.',
  }),
  rangeOf(DEMO_TRANSCRIPT, '2000 req/s', {
    verdict: 'unsupported',
    concept: 'throughput claim',
    explanation: 'A specific number with nothing behind it. How was it measured, and at what percentile?',
  }),
]

/**
 * The same demo, but written the way a real transcription renders speech — with
 * an accent, an em dash, a curly quote and an emoji — and carrying UTF-8 BYTE
 * offsets exactly as the Go backend sends them.
 *
 * This exists to exercise the byte→character conversion through the real
 * component rather than only in unit tests. Every span here sits after at least
 * one multi-byte character, so if the conversion regresses the highlights
 * visibly slide off their words instead of failing quietly.
 */
export const DEMO_UNICODE_TRANSCRIPT =
  'That was naïve — we didn’t have real flow control, so backpressure was just ' +
  'a bigger buffer, and it held 🔥 at 2000 req/s until the parameter server fell over.'

/** Byte offsets computed the way Go would, rather than hand-counted. */
function byteSpanOf(
  text: string,
  excerpt: string,
  span: Omit<ByteSpan, 'start' | 'end' | 'excerpt'>,
): ByteSpan {
  const at = text.indexOf(excerpt)
  if (at < 0) {
    throw new Error(`fixtures: excerpt not found: ${JSON.stringify(excerpt)}`)
  }
  const encoder = new TextEncoder()
  const start = encoder.encode(text.slice(0, at)).length
  return { start, end: start + encoder.encode(excerpt).length, excerpt, ...span }
}

export const DEMO_UNICODE_BYTE_SPANS: ByteSpan[] = [
  byteSpanOf(DEMO_UNICODE_TRANSCRIPT, 'real flow control', {
    verdict: 'validated',
    concept: 'flow control',
    explanation: 'Right diagnosis — naming the absence of flow control is the whole insight.',
  }),
  byteSpanOf(DEMO_UNICODE_TRANSCRIPT, 'backpressure was just a bigger buffer', {
    verdict: 'incorrect',
    concept: 'backpressure',
    explanation: 'A bigger buffer delays the problem. It is not flow control.',
    correction: 'Backpressure signals the producer to slow down.',
  }),
  byteSpanOf(DEMO_UNICODE_TRANSCRIPT, '2000 req/s', {
    verdict: 'unsupported',
    concept: 'throughput claim',
    explanation: 'A specific number with nothing behind it. Measured how, at what percentile?',
  }),
]

/** The verdict scale's worked examples, taught on the home page (§7.5). */
export const VERDICT_EXAMPLES: Record<Verdict, string> = {
  validated: 'used a bloom filter',
  incomplete: 'we cached the hot keys',
  unsupported: 'handled 2000 req/s',
  incorrect: 'backpressure is just a bigger buffer',
}
