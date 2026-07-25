/**
 * The difficulty band, and the one function that moves the room.
 *
 * The ambient thermal field, the width axis of the band numeral, and the
 * intensity of the blooms all read --heat-cold / --heat-hot / --heat-alpha /
 * --band-width, which the [data-band] blocks in tokens.css set. So driving the
 * entire signature is a single attribute write; the CSS does the rest, and the
 * registered properties make it travel rather than snap.
 */

/** Bands the backend can report. Its ladder clamps to 2..5. */
export type Band = 1 | 2 | 3 | 4 | 5

/** Band names, as surfaced in the `band` frame's `text` field. */
export const BAND_NAMES: Record<Band, string> = {
  1: 'Orientation',
  2: 'Application',
  3: 'Mechanism',
  4: 'Tradeoff',
  5: 'Adversarial',
}

export function clampBand(band: number): Band {
  if (!Number.isFinite(band)) return 3
  return Math.min(5, Math.max(1, Math.round(band))) as Band
}

/**
 * Drives the ambient field from session state. Call on every `band` frame.
 *
 * Out-of-range values are clamped rather than rejected: an unexpected band
 * should warm the room to its nearest stop, not leave it stuck at the previous
 * one.
 */
export function setBand(band: number): Band {
  const clamped = clampBand(band)
  document.documentElement.dataset.band = String(clamped)
  return clamped
}

/** Reads the band currently driving the field. */
export function currentBand(): Band {
  return clampBand(Number(document.documentElement.dataset.band))
}
