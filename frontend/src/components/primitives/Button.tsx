import type { ButtonHTMLAttributes, ReactNode } from 'react'

import s from './Button.module.css'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'hero' | 'standard' | 'compact'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /**
   * `primary` is the ember fill and there must be EXACTLY ONE per screen —
   * a screen with two primary actions has no primary action.
   */
  variant?: ButtonVariant
  size?: ButtonSize
  /** Stretch to the container's width. */
  block?: boolean
  /** Leading glyph. lucide-react at 20px, never an emoji. */
  icon?: ReactNode
}

export function Button({
  variant = 'secondary',
  size = 'standard',
  block = false,
  icon,
  children,
  type = 'button',
  ...rest
}: ButtonProps) {
  return (
    <button
      {...rest}
      type={type}
      className={[s.button, rest.className].filter(Boolean).join(' ')}
      data-variant={variant}
      data-size={size}
      data-block={block || undefined}
    >
      {icon}
      {children}
    </button>
  )
}

/**
 * Stacked control group, divided by hairlines rather than separated by gaps.
 * The Live Room's Hint / Done / Type / End cluster.
 */
export function ButtonGroup({ children }: { children: ReactNode }) {
  return <div className={s.group}>{children}</div>
}
