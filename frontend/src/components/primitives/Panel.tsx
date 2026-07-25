import type { HTMLAttributes, ReactNode } from 'react'

import s from './Panel.module.css'

// `title` is omitted from the DOM attributes deliberately: here it names the
// panel's header, not a browser tooltip.
interface PanelProps extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
  /** The uppercase mono header. Omit for an unlabelled surface. */
  title?: ReactNode
  /** Right-hand slot in the header: turn counters, timers, counts. */
  aside?: ReactNode
  /** Drop the body padding, for panels that wrap their own scroll region. */
  flush?: boolean
  /** Make the body scroll internally instead of growing the page. */
  scroll?: boolean
  children?: ReactNode
}

export function Panel({ title, aside, flush, scroll, children, ...rest }: PanelProps) {
  return (
    <section
      {...rest}
      className={[s.panel, rest.className].filter(Boolean).join(' ')}
      data-flush={flush || undefined}
      data-scroll={scroll || undefined}
    >
      {title !== undefined && (
        <h2 className={s.header}>
          <span>{title}</span>
          {aside !== undefined && <span className={s.aside}>{aside}</span>}
        </h2>
      )}
      <div className={s.body}>{children}</div>
    </section>
  )
}
