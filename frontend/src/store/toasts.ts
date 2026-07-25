import { create } from 'zustand'

/**
 * Transient notices. One consumer today — the band change — and the copy comes
 * from the backend's own `message` field rather than being written here.
 */
export interface Toast {
  id: number
  /** Optional accented lead line, e.g. "BAND 4 — TRADEOFF". */
  title?: string
  message: string
  /** Any colour token. Defaults to the primary action colour. */
  accent?: string
}

/** §8.6: the toast auto-dismisses at t=4200, having arrived at t=120. */
export const TOAST_TTL_MS = 4200

interface ToastState {
  toasts: Toast[]
  push: (toast: Omit<Toast, 'id'>) => number
  dismiss: (id: number) => void
  clear: () => void
}

let nextId = 1

export const useToasts = create<ToastState>((set) => ({
  toasts: [],

  push(toast) {
    const id = nextId++
    set((state) => ({ toasts: [...state.toasts, { ...toast, id }] }))

    // The timer lives here rather than in the component so a toast still
    // expires if its host unmounts and remounts mid-flight.
    setTimeout(() => {
      set((state) => ({ toasts: state.toasts.filter((t) => t.id !== id) }))
    }, TOAST_TTL_MS)

    return id
  },

  dismiss(id) {
    set((state) => ({ toasts: state.toasts.filter((t) => t.id !== id) }))
  },

  clear() {
    set({ toasts: [] })
  },
}))

/** Imperative entry point, for code that is not a React component. */
export function pushToast(toast: Omit<Toast, 'id'>): number {
  return useToasts.getState().push(toast)
}
