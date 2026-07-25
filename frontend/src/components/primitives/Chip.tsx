import type { HTMLAttributes, ReactNode } from 'react'

import { VERDICT_GLYPH, type Verdict } from '../../lib/verdict'
import s from './Chip.module.css'

type ChipTone = 'neutral' | Verdict

interface ChipProps extends HTMLAttributes<HTMLSpanElement> {
  tone?: ChipTone
  /** Wrap long concept names rather than overflowing the rail. */
  wrap?: boolean
  children?: ReactNode
}

export function Chip({ tone = 'neutral', wrap, children, ...rest }: ChipProps) {
  const glyph = tone === 'neutral' ? null : VERDICT_GLYPH[tone]

  return (
    <span
      {...rest}
      className={[s.chip, rest.className].filter(Boolean).join(' ')}
      data-tone={tone}
      data-wrap={wrap || undefined}
    >
      {glyph && (
        <span className={s.glyph} aria-hidden="true">
          {glyph}
        </span>
      )}
      {children}
    </span>
  )
}
