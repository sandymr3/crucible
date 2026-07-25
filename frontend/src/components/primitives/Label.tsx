import type { HTMLAttributes, ReactNode } from 'react'

import s from './Label.module.css'

interface LabelProps extends HTMLAttributes<HTMLSpanElement> {
  /** `quiet` is --dust — labels only, never body text (contrast §12). */
  tone?: 'default' | 'quiet' | 'loud' | 'accent'
  children?: ReactNode
}

export function Label({ tone = 'default', children, ...rest }: LabelProps) {
  return (
    <span
      {...rest}
      className={[s.label, rest.className].filter(Boolean).join(' ')}
      data-tone={tone === 'default' ? undefined : tone}
    >
      {children}
    </span>
  )
}

interface StatusLabelProps extends HTMLAttributes<HTMLSpanElement> {
  /** Any colour token, e.g. `var(--state-live)`. */
  color: string
  /**
   * Only true while the microphone is genuinely hot. The pulse means "we are
   * listening"; running it in another state makes it decoration and it stops
   * carrying information.
   */
  pulse?: boolean
  children?: ReactNode
}

/** A coloured dot plus a label — the status bar's state indicator. */
export function StatusLabel({ color, pulse, children, ...rest }: StatusLabelProps) {
  return (
    <span
      {...rest}
      className={[s.status, rest.className].filter(Boolean).join(' ')}
      style={{ color, ...rest.style }}
    >
      <span className={s.dot} data-pulse={pulse || undefined} aria-hidden="true" />
      <Label tone="accent">{children}</Label>
    </span>
  )
}
