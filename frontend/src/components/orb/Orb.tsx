import { useId, type CSSProperties, type Ref } from 'react'

import { personaAccent, type PersonaId } from '../../lib/persona'
import type { LiveState } from '../../lib/protocol'
import s from './Orb.module.css'

/**
 * SVG user units. The core is r=64; the rings sit at 100%, 128% and 156% of it,
 * so the outermost reaches r=100 and the 208-unit box leaves it a hair of
 * margin for its stroke.
 */
const BOX = 208
const CENTER = BOX / 2
const CORE_R = 64
const RING_R = [64, 82, 100]

interface OrbProps {
  state: LiveState
  persona?: PersonaId | null
  /**
   * Diameter of the CORE in pixels — the part that reads as "the orb". The
   * rings extend past it as a halo, so the element's own box is larger.
   */
  size?: number
  /**
   * Attach useOrbEnergy's ref here to drive amplitude. Left unattached the orb
   * simply rests at zero energy, which is a valid state rather than a broken
   * one.
   */
  ref?: Ref<HTMLDivElement>
  style?: CSSProperties
}

/**
 * The visual anchor of the product.
 *
 * Amplitude is NOT a prop. It arrives per audio chunk and is written straight
 * onto --orb-energy by useOrbEnergy, so a fifty-times-a-second signal never
 * enters React's render path.
 *
 * aria-hidden: it carries no information a screen reader can use that the
 * status label does not already announce.
 */
export function Orb({ state, persona, size = 132, ref, style }: OrbProps) {
  const gradientId = useId()
  const box = (size * BOX) / (CORE_R * 2)

  return (
    <div
      ref={ref}
      className={s.orb}
      data-state={state}
      aria-hidden="true"
      style={{
        width: box,
        height: box,
        ['--orb-accent' as string]: personaAccent(persona),
        ...style,
      }}
    >
      <svg className={s.svg} viewBox={`0 0 ${BOX} ${BOX}`} role="presentation">
        <defs>
          <radialGradient id={gradientId}>
            <stop offset="0%" stopColor="var(--orb-accent)" stopOpacity="0.45" />
            <stop offset="100%" stopColor="var(--orb-accent)" stopOpacity="0" />
          </radialGradient>
        </defs>

        <g className={s.glow}>
          <circle
            className={s.core}
            cx={CENTER}
            cy={CENTER}
            r={CORE_R}
            fill="var(--vessel-high)"
          />
          <circle
            className={s.core}
            cx={CENTER}
            cy={CENTER}
            r={CORE_R}
            fill={`url(#${gradientId})`}
          />
        </g>

        <g className={s.rings}>
          {RING_R.map((r, i) => (
            <circle
              key={r}
              className={`${s.ring} ${[s.ring1, s.ring2, s.ring3][i]}`}
              cx={CENTER}
              cy={CENTER}
              r={r}
            />
          ))}
        </g>
      </svg>
    </div>
  )
}
