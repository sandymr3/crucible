/**
 * Motion timings for the two moments that matter (design PRD §8.5, §8.6).
 *
 * Held in one place rather than restated at each call site: the home page's
 * hero demo and the Live Room must run the identical reveal, and the band
 * change is a single event expressed across four channels that have to line up.
 */

/** How long one span takes to draw its rule and fade in its tint. */
export const SPAN_REVEAL_MS = 320

/** The gap between consecutive spans, left to right. */
export const SPAN_STAGGER_MS = 160

/** How long the whole sequence takes for a given number of spans. */
export function revealDuration(spanCount: number): number {
  return spanCount <= 0 ? 0 : SPAN_STAGGER_MS * (spanCount - 1) + SPAN_REVEAL_MS
}

/**
 * The band change (§8.6), in order:
 *
 *   t=0     the thermal properties begin their 1800ms transition, and the
 *           width axis begins widening over 900ms — same event, two channels,
 *           the faster one landing first
 *   t=120   the toast arrives, and a single flare sweeps the band indicator
 *   t=4200  the toast dismisses itself
 *
 * The 120ms offset is what makes this read as a consequence rather than a
 * coincidence: the room starts moving first, and the explanation follows it.
 *
 * Deliberately silent. The AI is already talking, and a notification chime
 * over a voice interface is jarring.
 */
export const BAND_THERMAL_MS = 1800
export const BAND_WIDTH_MS = 900
export const BAND_ANNOUNCE_DELAY_MS = 120
export const BAND_FLARE_MS = 400
