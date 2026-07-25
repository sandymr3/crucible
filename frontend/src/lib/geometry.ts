/**
 * Chart geometry.
 *
 * Pure, so the shapes can be asserted without rendering anything. All three
 * visualisations are hand-rolled SVG rather than a charting library: the radar
 * is a polygon, the sparkline a polyline, and the dial an arc, and matching the
 * design system exactly — a 1px --rim grid, a 14% fill, per-segment stroke
 * colours — is less work than overriding a library's defaults into submission.
 */

export interface Point {
  x: number
  y: number
}

/**
 * Polar to cartesian in SVG's coordinate space.
 *
 * Note y grows DOWNWARD, so a positive angle sweeps clockwise on screen. Angle
 * 0 points east; -90 points north, which is where a radar's first axis belongs.
 */
export function polar(cx: number, cy: number, radius: number, degrees: number): Point {
  const radians = (degrees * Math.PI) / 180
  return {
    x: cx + radius * Math.cos(radians),
    y: cy + radius * Math.sin(radians),
  }
}

/** Evenly spaced axis angles, first one pointing straight up. */
export function axisAngles(count: number): number[] {
  if (count <= 0) return []
  return Array.from({ length: count }, (_, i) => -90 + (360 / count) * i)
}

/**
 * The vertices of a radar series.
 *
 * `max` is the top of the scale rather than the largest value present: a radar
 * normalised to its own maximum always looks full, which would make a weak
 * session and a strong one identical.
 */
export function radarPoints(
  values: number[],
  cx: number,
  cy: number,
  radius: number,
  max: number,
): Point[] {
  const angles = axisAngles(values.length)
  return values.map((value, i) => {
    const clamped = Math.min(max, Math.max(0, value))
    return polar(cx, cy, (clamped / max) * radius, angles[i])
  })
}

export function toPath(points: Point[], close = false): string {
  if (points.length === 0) return ''
  const body = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`)
  return body.join(' ') + (close ? ' Z' : '')
}

/**
 * An SVG arc path from one angle to another, sweeping clockwise.
 *
 * The large-arc flag is computed rather than hardcoded, because the pace dial's
 * 220-degree sweep crosses the 180-degree boundary and a fixed flag would draw
 * the complementary arc — the 140-degree gap instead of the dial.
 */
export function arcPath(
  cx: number,
  cy: number,
  radius: number,
  startDeg: number,
  endDeg: number,
): string {
  const start = polar(cx, cy, radius, startDeg)
  const end = polar(cx, cy, radius, endDeg)
  const largeArc = Math.abs(endDeg - startDeg) > 180 ? 1 : 0
  const sweep = endDeg > startDeg ? 1 : 0
  return (
    `M${start.x.toFixed(2)},${start.y.toFixed(2)} ` +
    `A${radius},${radius} 0 ${largeArc} ${sweep} ${end.x.toFixed(2)},${end.y.toFixed(2)}`
  )
}

/** Maps a value in [min,max] onto a position in [0,1], clamped. */
export function normalise(value: number, min: number, max: number): number {
  if (max === min) return 0
  return Math.min(1, Math.max(0, (value - min) / (max - min)))
}
