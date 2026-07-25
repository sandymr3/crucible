import type { HTMLAttributes } from 'react'

import { truncateWords } from '../../lib/text'
import { VERDICT_GLYPH, VERDICT_NAME, verdictColor, type Verdict } from '../../lib/verdict'
import s from './Popover.module.css'

/** Hard cap from §9.4 — truncate rather than overflow. */
const MAX_WORDS = 40

interface PopoverProps extends HTMLAttributes<HTMLDivElement> {
  verdict: Verdict
  concept: string
  explanation: string
  /** Present for `incorrect` and `incomplete`: what the right answer looks like. */
  correction?: string
}

/**
 * The explanation attached to a graded span.
 *
 * Presentational only — positioning belongs to whatever anchors it, because the
 * transcript is the only thing that knows where its spans landed.
 */
export function Popover({ verdict, concept, explanation, correction, ...rest }: PopoverProps) {
  return (
    <div
      {...rest}
      role="tooltip"
      className={[s.popover, rest.className].filter(Boolean).join(' ')}
      style={{ ['--popover-accent' as string]: verdictColor(verdict), ...rest.style }}
    >
      <div className={s.concept}>
        <span className={s.glyph} aria-hidden="true">
          {VERDICT_GLYPH[verdict]}
        </span>
        <span className="sr-only">{VERDICT_NAME[verdict]}: </span>
        {concept}
      </div>
      <p className={s.explanation}>{truncateWords(explanation, MAX_WORDS)}</p>
      {correction && <p className={s.correction}>{truncateWords(correction, MAX_WORDS)}</p>}
    </div>
  )
}
