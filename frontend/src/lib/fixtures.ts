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

/** The verdict scale's worked examples, taught on the home page (§7.5). */
export const VERDICT_EXAMPLES: Record<Verdict, string> = {
  validated: 'used a bloom filter',
  incomplete: 'we cached the hot keys',
  unsupported: 'handled 2000 req/s',
  incorrect: 'backpressure is just a bigger buffer',
}
