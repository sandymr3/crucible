import { clampBand } from '../../lib/band'
import { Label } from '../primitives/Label'
import s from './Charts.module.css'

/** Band to thermal ramp stop. The same mapping the ambient field uses. */
const BAND_COLOR: Record<number, string> = {
  1: 'var(--t-quench)',
  2: 'var(--t-cool)',
  3: 'var(--t-temper)',
  4: 'var(--t-warm)',
  5: 'var(--t-ember)',
}

interface BandSparklineProps {
  /** Band at each point, in order. */
  trajectory: number[]
  width?: number
  height?: number
}

/**
 * The difficulty trajectory.
 *
 * Each segment is stroked by the band it climbs TO, so the line itself warms
 * left to right — the thermal ramp doing a fourth job, which is what ties the
 * report back to the room it came from.
 *
 * Per-segment colour is why this is hand-rolled: it is one polyline in every
 * charting library, and one stroke colour for the whole line loses the point.
 */
export function BandSparkline({ trajectory, width = 260, height = 56 }: BandSparklineProps) {
  const points = trajectory.map(clampBand)

  if (points.length === 0) {
    return (
      <div className={s.empty}>
        <Label tone="quiet">The difficulty has not moved yet</Label>
      </div>
    )
  }

  const pad = 6
  // Bands run 2-5 in practice, but the axis covers 1-5 so a demotion to the
  // floor still has somewhere to sit.
  const lo = 1
  const hi = 5
  const usableW = width - pad * 2
  const usableH = height - pad * 2

  const xAt = (i: number) =>
    points.length === 1 ? width / 2 : pad + (usableW * i) / (points.length - 1)
  const yAt = (band: number) => pad + usableH * (1 - (band - lo) / (hi - lo))

  return (
    <svg
      className={s.chart}
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={`Difficulty went ${points.join(' to ')}`}
    >
      <line
        className={s.sparkBaseline}
        x1={pad}
        y1={height - pad}
        x2={width - pad}
        y2={height - pad}
      />

      {points.slice(0, -1).map((band, i) => {
        const next = points[i + 1]
        return (
          <line
            key={i}
            className={s.sparkSegment}
            x1={xAt(i)}
            y1={yAt(band)}
            x2={xAt(i + 1)}
            y2={yAt(next)}
            stroke={BAND_COLOR[next]}
          />
        )
      })}

      {points.map((band, i) => (
        <circle
          key={i}
          className={s.sparkNode}
          cx={xAt(i)}
          cy={yAt(band)}
          r={i === points.length - 1 ? 4 : 2.5}
          fill={BAND_COLOR[band]}
        />
      ))}
    </svg>
  )
}
