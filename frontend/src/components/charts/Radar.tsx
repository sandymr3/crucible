import { axisAngles, polar, radarPoints, toPath } from '../../lib/geometry'
import { Label } from '../primitives/Label'
import s from './Charts.module.css'

/** Rings at 25 / 50 / 75 / 100 percent. */
const RINGS = [0.25, 0.5, 0.75, 1]

/**
 * Turns needed before the shape means anything.
 *
 * Under three graded turns the radar is flat by design — every axis carries the
 * same one or two scores — and a flat shape reads as a rendering bug rather
 * than as "not enough data". §9.3 calls for an explicit state instead.
 */
export const RADAR_MIN_TURNS = 3

export interface RadarAxis {
  label: string
  value: number
}

interface RadarProps {
  axes: RadarAxis[]
  /** Top of the scale. Rubric dimensions are 1-10; domain scores are 0-10. */
  max?: number
  /** Graded turns so far, for the under-three state. */
  turnsGraded?: number
  size?: number
}

/**
 * The score matrix.
 *
 * No legend: a single series needs no key, and a legend on a four-axis radar is
 * pure decoration.
 */
export function Radar({ axes, max = 10, turnsGraded, size = 220 }: RadarProps) {
  if (axes.length < 3) {
    return (
      <div className={s.empty}>
        <Label tone="quiet">Not enough dimensions to plot</Label>
      </div>
    )
  }

  if (turnsGraded !== undefined && turnsGraded < RADAR_MIN_TURNS) {
    const remaining = RADAR_MIN_TURNS - turnsGraded
    return (
      <div className={s.empty}>
        <Label tone="quiet">
          {remaining} more answer{remaining === 1 ? '' : 's'} before the shape means anything
        </Label>
      </div>
    )
  }

  // Room for the labels, which sit outside the outermost ring.
  const pad = 26
  const cx = size / 2
  const cy = size / 2
  const radius = size / 2 - pad

  const angles = axisAngles(axes.length)
  const points = radarPoints(
    axes.map((a) => a.value),
    cx,
    cy,
    radius,
    max,
  )

  return (
    <svg
      className={s.chart}
      viewBox={`0 0 ${size} ${size}`}
      role="img"
      aria-label={axes.map((a) => `${a.label} ${a.value.toFixed(1)} out of ${max}`).join(', ')}
    >
      {RINGS.map((ring) => (
        <path
          key={ring}
          className={s.grid}
          d={toPath(
            angles.map((angle) => polar(cx, cy, radius * ring, angle)),
            true,
          )}
        />
      ))}

      {angles.map((angle, i) => {
        const outer = polar(cx, cy, radius, angle)
        return (
          <line key={i} className={s.grid} x1={cx} y1={cy} x2={outer.x} y2={outer.y} />
        )
      })}

      <path className={s.radarArea} d={toPath(points, true)} />

      {points.map((point, i) => (
        <circle key={i} className={s.radarVertex} cx={point.x} cy={point.y} r={3} />
      ))}

      {angles.map((angle, i) => {
        const at = polar(cx, cy, radius + 14, angle)
        // Anchor by hemisphere so labels lean away from the shape rather than
        // sitting on top of it.
        const anchor = at.x > cx + 1 ? 'start' : at.x < cx - 1 ? 'end' : 'middle'
        return (
          <text
            key={i}
            className={s.axisLabel}
            x={at.x}
            y={at.y}
            textAnchor={anchor}
            dominantBaseline="middle"
          >
            {axes[i].label}
          </text>
        )
      })}
    </svg>
  )
}
