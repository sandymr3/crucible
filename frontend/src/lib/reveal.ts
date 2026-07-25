/**
 * Reveal timings (design PRD §8.5).
 *
 * The heatmap reveal is one of the three moments that matter, so its numbers
 * live here rather than being restated at each call site — the home page's hero
 * demo and the Live Room must run the identical sequence.
 */

/** How long one span takes to draw its rule and fade in its tint. */
export const SPAN_REVEAL_MS = 320

/** The gap between consecutive spans, left to right. */
export const SPAN_STAGGER_MS = 160

/** How long the whole sequence takes for a given number of spans. */
export function revealDuration(spanCount: number): number {
  return spanCount <= 0 ? 0 : SPAN_STAGGER_MS * (spanCount - 1) + SPAN_REVEAL_MS
}
