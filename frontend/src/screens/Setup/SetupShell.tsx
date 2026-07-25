import type { ReactNode } from 'react'

import { Label } from '../../components/primitives'
import s from './Setup.module.css'

export type SetupStep = 'mode' | 'material' | 'digest' | 'persona' | 'plan'

const ORDER: { key: SetupStep; label: string }[] = [
  { key: 'mode', label: 'Mode' },
  { key: 'material', label: 'Material' },
  { key: 'digest', label: 'Plan' },
  { key: 'persona', label: 'Interviewer' },
  { key: 'plan', label: 'Focus' },
]

/**
 * The frame every setup screen sits in.
 *
 * The step row exists so the flow reads as short and finite: the product is the
 * conversation, and everything here is between the candidate and it.
 */
export function SetupShell({
  step,
  title,
  lede,
  wide,
  children,
}: {
  step: SetupStep
  title: string
  lede?: ReactNode
  wide?: boolean
  children: ReactNode
}) {
  const current = ORDER.findIndex((entry) => entry.key === step)

  return (
    <div className={`${s.page} ${wide ? s.wide : ''}`}>
      <nav className={s.steps} aria-label="Setup progress">
        {ORDER.map((entry, i) => (
          <span key={entry.key} style={{ display: 'contents' }}>
            {i > 0 && <span className={s.stepSep} aria-hidden="true" />}
            <span
              className={`${s.step} ${
                i === current ? s.stepActive : i < current ? s.stepDone : ''
              }`}
              aria-current={i === current ? 'step' : undefined}
            >
              <span className={s.stepDot} aria-hidden="true" />
              <Label tone="accent">{entry.label}</Label>
            </span>
          </span>
        ))}
      </nav>

      <div>
        <h1 className={s.title}>{title}</h1>
        {lede && <p className={s.lede}>{lede}</p>}
      </div>

      {children}
    </div>
  )
}
