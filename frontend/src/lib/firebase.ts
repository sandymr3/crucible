import { initializeApp, type FirebaseApp } from 'firebase/app'
import {
  GoogleAuthProvider,
  getAuth,
  getRedirectResult,
  linkWithPopup,
  onAuthStateChanged,
  signInAnonymously,
  signInWithCredential,
  signInWithPopup,
  signInWithRedirect,
  signOut,
  type Auth,
  type AuthError,
  type User,
} from 'firebase/auth'

/**
 * Firebase Auth. The backend verifies every request against this project, so
 * there is no path into /v1 without a real ID token — DEV_ALLOW_ANON is refused
 * on Cloud Run precisely because an unauthenticated socket in front of a
 * billing API is a credit leak that only has to be found once.
 *
 * The web API key here is not a secret. It identifies the project; it
 * authorises nothing. Access is governed by Auth and by the Firestore rules in
 * backend/deploy/firestore.rules, which make the backend the only writer.
 */

const config = {
  apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
  authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
  appId: import.meta.env.VITE_FIREBASE_APP_ID,
  storageBucket: import.meta.env.VITE_FIREBASE_STORAGE_BUCKET,
  messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
}

/**
 * Whether the app can authenticate at all.
 *
 * Checked rather than assumed so the home page still renders on a machine with
 * no .env.local. A landing page that white-screens because a config value is
 * missing is a worse failure than one that cannot sign you in.
 */
export function isFirebaseConfigured(): boolean {
  return Boolean(config.apiKey && config.authDomain && config.projectId && config.appId)
}

export class FirebaseNotConfiguredError extends Error {
  constructor() {
    super(
      'Firebase is not configured. Copy .env.example to .env.local and fill in ' +
        'the VITE_FIREBASE_* values from the Firebase console.',
    )
    this.name = 'FirebaseNotConfiguredError'
  }
}

let app: FirebaseApp | null = null
let auth: Auth | null = null

/** Lazy so an unconfigured build still loads and renders. */
function ensureAuth(): Auth {
  if (!isFirebaseConfigured()) throw new FirebaseNotConfiguredError()
  if (!auth) {
    app = app ?? initializeApp(config)
    auth = getAuth(app)
  }
  return auth
}

export type AuthUser = User

let authReady: Promise<void> | null = null

/**
 * Resolves once the SDK has restored (or ruled out) a persisted user. On a
 * cold load `currentUser` is null for the first few hundred milliseconds while
 * IndexedDB is read; anything that reads it before then sees "signed out" for
 * a user who is signed in.
 */
function whenAuthReady(): Promise<void> {
  if (!isFirebaseConfigured()) return Promise.resolve()
  if (!authReady) {
    authReady = new Promise((resolve) => {
      const unsubscribe = onAuthStateChanged(ensureAuth(), () => {
        resolve()
        unsubscribe()
      })
    })
  }
  return authReady
}

/**
 * Subscribes to sign-in state. Returns an unsubscribe function.
 *
 * When Firebase is unconfigured this reports "signed out" once and stops,
 * rather than throwing — callers should render a signed-out UI, not crash.
 */
export function onAuthChange(callback: (user: User | null) => void): () => void {
  if (!isFirebaseConfigured()) {
    callback(null)
    return () => {}
  }
  return onAuthStateChanged(ensureAuth(), callback)
}

/**
 * Popup errors that mean the POPUP MECHANISM failed, not the sign-in itself.
 * For these the redirect flow is the correct retry, immediately.
 */
const POPUP_MECHANISM_FAILURES = [
  'auth/popup-blocked',
  'auth/web-storage-unsupported',
  'auth/operation-not-supported-in-this-environment',
]

/**
 * Google sign-in. Popup first; falls back to redirect when the popup path is
 * unavailable. Resolves null when a redirect navigation has started — the
 * result arrives via completeRedirectSignIn() after the round trip.
 *
 * A closed popup is a cancel, nothing more: the error is rethrown and the next
 * attempt opens a fresh popup. It must never demote future attempts to the
 * redirect flow — storage partitioning against the auth domain makes redirect
 * sign-in silently drop the result on modern browsers.
 *
 * A guest who signs in keeps their sessions: linking upgrades the anonymous
 * account in place, preserving the UID the backend keys everything by.
 */
export async function signInWithGoogle(): Promise<User | null> {
  const provider = new GoogleAuthProvider()
  const authInstance = ensureAuth()
  const anon = authInstance.currentUser?.isAnonymous ? authInstance.currentUser : null

  try {
    const result = anon
      ? await linkWithPopup(anon, provider)
      : await signInWithPopup(authInstance, provider)
    return result.user
  } catch (error) {
    const code = (error as { code?: string }).code ?? ''
    // The Google account already exists as its own user, so linking is
    // impossible. Sign into that account instead; the guest's sessions stay
    // on the abandoned anonymous UID, which beats failing the sign-in.
    if (anon && (code.includes('credential-already-in-use') || code.includes('email-already-in-use'))) {
      const credential = GoogleAuthProvider.credentialFromError(error as AuthError)
      const result = credential
        ? await signInWithCredential(authInstance, credential)
        : await signInWithPopup(authInstance, provider)
      return result.user
    }
    if (POPUP_MECHANISM_FAILURES.some((c) => code.includes(c))) {
      await signInWithRedirect(authInstance, provider)
      return null
    }
    throw error
  }
}

/**
 * Completes a redirect-flow sign-in after the round trip back to the app.
 * Resolves null when there was no pending redirect. Called once at startup.
 */
export function completeRedirectSignIn(): Promise<User | null> {
  if (!isFirebaseConfigured()) return Promise.resolve(null)
  return getRedirectResult(ensureAuth()).then((result) => result?.user ?? null)
}

/**
 * Anonymous sign-in. The backend reports it as `anonymous: true` on /v1/me and
 * treats the session identically — a candidate should not have to hand over an
 * identity to rehearse an interview.
 */
export function signInAnon(): Promise<User> {
  return signInAnonymously(ensureAuth()).then((result) => result.user)
}

export function signOutUser(): Promise<void> {
  return signOut(ensureAuth())
}

/**
 * The current ID token, or null when signed out.
 *
 * The SDK refreshes on its own as expiry approaches, so this is called per
 * request rather than cached. Caching it is how a long interview ends with a
 * sudden 401 partway through.
 *
 * Waits for the persisted user to be restored first: a request fired during
 * the first render of a cold load would otherwise go out unauthenticated and
 * 401 for someone who is signed in.
 */
export async function getIdToken(): Promise<string | null> {
  if (!isFirebaseConfigured()) return null
  await whenAuthReady()
  const user = ensureAuth().currentUser
  return user ? user.getIdToken() : null
}

/** Current user without subscribing. */
export function currentUser(): User | null {
  return isFirebaseConfigured() ? ensureAuth().currentUser : null
}
