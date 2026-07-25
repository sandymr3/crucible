import { initializeApp, type FirebaseApp } from 'firebase/app'
import {
  GoogleAuthProvider,
  getAuth,
  onAuthStateChanged,
  signInAnonymously,
  signInWithPopup,
  signOut,
  type Auth,
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

export function signInWithGoogle(): Promise<User> {
  const provider = new GoogleAuthProvider()
  return signInWithPopup(ensureAuth(), provider).then((result) => result.user)
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
 */
export async function getIdToken(): Promise<string | null> {
  if (!isFirebaseConfigured()) return null
  const user = ensureAuth().currentUser
  return user ? user.getIdToken() : null
}

/** Current user without subscribing. */
export function currentUser(): User | null {
  return isFirebaseConfigured() ? ensureAuth().currentUser : null
}
