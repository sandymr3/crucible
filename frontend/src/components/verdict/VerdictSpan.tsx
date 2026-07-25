import type { HTMLAttributes, ReactNode } from 'react'

import { VERDICT_GLYPH, VERDICT_NAME, type Verdict } from '../../lib/verdict'
import s from './VerdictSpan.module.css'

interface VerdictSpanProps extends HTMLAttributes<HTMLSpanElement> {
  verdict: Verdict
  /** Named in the accessible label, so the finding survives without colour. */
  concept: string
  /** True while the reveal sequence is running. */
  revealing?: boolean
  /** Position in the stagger, in milliseconds. */
  delay?: number
  children: ReactNode
}

/**
 * One graded stretch of the transcript.
 *
 * ONE component, used by both the Live Room and the home page's hero demo.
 * Building it twice is how the two drift apart and the home page starts lying
 * about the product.
 *
 * Focusable on purpose: the explanation is attached to hover, and a keyboard
 * user must be able to reach it too.
 */
export function VerdictSpan({
  verdict,
  concept,
  revealing,
  delay = 0,
  children,
  ...rest
}: VerdictSpanProps) {
  return (
    <span
      {...rest}
      className={[s.span, rest.className].filter(Boolean).join(' ')}
      data-verdict={verdict}
      data-reveal={revealing || undefined}
      style={{ ['--span-delay' as string]: `${delay}ms`, ...rest.style }}
      tabIndex={0}
      role="mark"
      aria-label={`${VERDICT_NAME[verdict]}: ${concept}`}
    >
      {children}
      <span className={s.glyph} aria-hidden="true">
        {VERDICT_GLYPH[verdict]}
      </span>
    </span>
  )
}
