import { create } from 'zustand'

import {
  completeRedirectSignIn,
  isFirebaseConfigured,
  onAuthChange,
  signInAnon,
  signInWithGoogle,
  signOutUser,
  type AuthUser,
} from '../lib/firebase'
import { pushToast } from './toasts'

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
    if (error.message.includes('popup-closed-by-user'))
      return 'The sign-in window closed early. Click Sign in again — the retry uses a full-page redirect instead.'
    if (error.message.includes('popup-blocked')) return 'Your browser blocked the sign-in popup.'
    if (error.message.includes('unauthorized-domain'))
      return 'This domain is not authorised for sign-in. (Firebase console → Authentication → Settings → Authorized domains.)'
    if (error.message.includes('operation-not-allowed'))
      return 'Google sign-in is not enabled for this project. (Firebase console → Authentication → Sign-in method.)'
    if (error.message.includes('network-request-failed')) return 'Network error. Try again.'
    return error.message
  }
  return 'Sign-in failed.'
}

/** Set the error state AND say it out loud — a silent auth failure looks like
    the popup "just closed" for no reason, which reads as a broken app. */
function reportAuthError(error: unknown) {
  const message = messageOf(error)
  useAuth.setState({ error: message })
  pushToast({ title: 'SIGN-IN FAILED', message, accent: 'var(--t-assay)' })
}

export const useAuth = create<AuthState>((set) => ({
  user: null,
  loading: isFirebaseConfigured(),
  error: null,
  configured: isFirebaseConfigured(),

  async signInGoogle() {
    set({ error: null })
    try {
      // Resolves null when the redirect flow has taken over — the page is
      // about to navigate away, so there is nothing further to do here.
      await signInWithGoogle()
    } catch (error) {
      reportAuthError(error)
    }
  },

  async signInGuest() {
    set({ error: null })
    try {
      await signInAnon()
    } catch (error) {
      reportAuthError(error)
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

/**
 * Complete a pending redirect-flow sign-in, once, at startup. A failure here
 * is the redirect flow's equivalent of the popup closing — it must be said
 * out loud or the round trip appears to have done nothing.
 */
completeRedirectSignIn().catch(reportAuthError)
