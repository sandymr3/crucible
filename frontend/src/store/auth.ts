import { create } from 'zustand'

import {
  isFirebaseConfigured,
  onAuthChange,
  signInAnon,
  signInWithGoogle,
  signOutUser,
  type AuthUser,
} from '../lib/firebase'

interface AuthState {
  user: AuthUser | null
  /** True until the first auth state callback lands. */
  loading: boolean
  error: string | null
  /** False when VITE_FIREBASE_* is absent — render signed-out, do not crash. */
  configured: boolean

  signInGoogle: () => Promise<void>
  signInGuest: () => Promise<void>
  signOut: () => Promise<void>
  clearError: () => void
}

function messageOf(error: unknown): string {
  if (error instanceof Error) {
    // Firebase's own copy is opaque ("auth/popup-closed-by-user"), so the few
    // a user can act on are translated and the rest fall through.
    if (error.message.includes('popup-closed-by-user')) return 'Sign-in was cancelled.'
    if (error.message.includes('popup-blocked')) return 'Your browser blocked the sign-in popup.'
    if (error.message.includes('network-request-failed')) return 'Network error. Try again.'
    return error.message
  }
  return 'Sign-in failed.'
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  loading: isFirebaseConfigured(),
  error: null,
  configured: isFirebaseConfigured(),

  async signInGoogle() {
    set({ error: null })
    try {
      await signInWithGoogle()
    } catch (error) {
      set({ error: messageOf(error) })
    }
  },

  async signInGuest() {
    set({ error: null })
    try {
      await signInAnon()
    } catch (error) {
      set({ error: messageOf(error) })
    }
  },

  async signOut() {
    try {
      await signOutUser()
    } catch (error) {
      set({ error: messageOf(error) })
    }
  },

  clearError: () => set({ error: null }),
}))

/**
 * Subscribed once at module load rather than from a component effect.
 *
 * Auth state is global and outlives any single tree, and a StrictMode double
 * mount would otherwise subscribe twice.
 */
onAuthChange((user) => {
  useAuth.setState({ user, loading: false })
})
