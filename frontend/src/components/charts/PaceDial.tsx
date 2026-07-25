import { arcPath, normalise, polar } from '../../lib/geometry'
import s from './Charts.module.css'

/**
 * A 220-degree sweep with the gap at the bottom. In SVG's coordinate space y
 * grows downward, so increasing the angle sweeps clockwise on screen: 160 to
 * 380 passes through the top and leaves 140 degrees open beneath.
 */
const START_DEG = 160
const SWEEP_DEG = 220

/** The scale. Wide enough that a genuinely fast talker still lands on it. */
const MIN_WPM = 60
const MAX_WPM = 220

/**
 * The optimal zone, shown as a band rather than a target.
 *
 * Optimal pace is context-dependent — a system-design walkthrough should be
 * slower than a behavioural story — so this is guidance and never a grade.
 */
const OPTIMAL = { from: 110, to: 160 }

function angleFor(wpm: number): number {
  return START_DEG + SWEEP_DEG * normalise(wpm, MIN_WPM, MAX_WPM)
}

interface PaceDialProps {
  wpm: number
  /** The backend's own classification: hesitant / optimal / rushed / too fast. */
  band?: string
  size?: number
}

export function PaceDial({ wpm, band, size = 180 }: PaceDialProps) {
  const cx = size / 2
  const cy = size / 2
  const radius = size / 2 - 16

  const needleOuter = polar(cx, cy, radius - 4, angleFor(wpm))
  const needleInner = polar(cx, cy, radius * 0.42, angleFor(wpm))

  return (
    <svg
      className={s.chart}
      viewBox={`0 0 ${size} ${size * 0.82}`}
      role="img"
      aria-label={`${Math.round(wpm)} words per minute${band ? `, ${band}` : ''}`}
    >
      <path
        className={s.dialTrack}
        d={arcPath(cx, cy, radius, START_DEG, START_DEG + SWEEP_DEG)}
      />
      <path
        className={s.dialZone}
        d={arcPath(cx, cy, radius, angleFor(OPTIMAL.from), angleFor(OPTIMAL.to))}
      />

      {wpm > 0 && (
        <line
          className={s.dialNeedle}
          x1={needleInner.x}
          y1={needleInner.y}
          x2={needleOuter.x}
          y2={needleOuter.y}
        />
      )}

      <text className={s.dialValue} x={cx} y={cy + 4}>
        {wpm > 0 ? Math.round(wpm) : '—'}
      </text>
      <text className={s.dialUnit} x={cx} y={cy + 22}>
        {band ?? 'words / min'}
      </text>
    </svg>
  )
}
