import { X } from 'lucide-react'

import { useToasts } from '../../store/toasts'
import { Label } from './Label'
import s from './Toast.module.css'

/**
 * Fixed top-right stack. Mount once at the app root.
 *
 * `aria-live="polite"` rather than `assertive`: a band change is worth
 * announcing, but not worth interrupting the interviewer mid-sentence.
 */
export function ToastHost() {
  const toasts = useToasts((state) => state.toasts)
  const dismiss = useToasts((state) => state.dismiss)

  return (
    <div className={s.host} aria-live="polite" aria-atomic="false">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={s.toast}
          style={toast.accent ? { ['--toast-accent' as string]: toast.accent } : undefined}
        >
          <div>
            {toast.title && (
              <Label tone="accent" className={s.title}>
                {toast.title}
              </Label>
            )}
            {toast.message}
          </div>
          <button
            type="button"
            className={s.dismiss}
            onClick={() => dismiss(toast.id)}
            aria-label="Dismiss"
          >
            <X size={16} strokeWidth={1.5} />
          </button>
        </div>
      ))}
    </div>
  )
}
