import { beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * Exercises the sign-in decision tree with the SDK mocked out: which of
 * popup / link / credential / redirect gets called, and when.
 *
 * The regressions this guards against were user-facing: a module-level latch
 * once demoted every sign-in after a closed popup to the redirect flow (which
 * silently drops its result on browsers that partition storage against the
 * auth domain), and signing in used to replace a guest's UID instead of
 * linking, orphaning every session they had created.
 */

const mocks = vi.hoisted(() => {
  const authInstance = { currentUser: null as null | { isAnonymous: boolean } }
  return {
    authInstance,
    getAuth: vi.fn(() => authInstance),
    onAuthStateChanged: vi.fn((_auth: unknown, callback: (user: unknown) => void) => {
      callback(authInstance.currentUser)
      return () => {}
    }),
    signInWithPopup: vi.fn(),
    signInWithRedirect: vi.fn(),
    signInWithCredential: vi.fn(),
    linkWithPopup: vi.fn(),
    credentialFromError: vi.fn(),
  }
})

vi.mock('firebase/app', () => ({ initializeApp: vi.fn(() => ({})) }))

vi.mock('firebase/auth', () => {
  class GoogleAuthProvider {
    static credentialFromError = mocks.credentialFromError
  }
  return {
    GoogleAuthProvider,
    getAuth: mocks.getAuth,
    getRedirectResult: vi.fn(() => Promise.resolve(null)),
    linkWithPopup: mocks.linkWithPopup,
    onAuthStateChanged: mocks.onAuthStateChanged,
    signInAnonymously: vi.fn(),
    signInWithCredential: mocks.signInWithCredential,
    signInWithPopup: mocks.signInWithPopup,
    signInWithRedirect: mocks.signInWithRedirect,
    signOut: vi.fn(),
  }
})

/** Fresh module per test: firebase.ts captures env and state at load. */
async function loadFirebase() {
  vi.stubEnv('VITE_FIREBASE_API_KEY', 'test-key')
  vi.stubEnv('VITE_FIREBASE_AUTH_DOMAIN', 'test.firebaseapp.com')
  vi.stubEnv('VITE_FIREBASE_PROJECT_ID', 'test-project')
  vi.stubEnv('VITE_FIREBASE_APP_ID', 'test-app')
  vi.resetModules()
  return import('./firebase')
}

function firebaseError(code: string): Error {
  const error = new Error(code) as Error & { code: string }
  error.code = code
  return error
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.authInstance.currentUser = null
})

describe('signInWithGoogle', () => {
  it('uses a popup when signed out', async () => {
    const { signInWithGoogle } = await loadFirebase()
    mocks.signInWithPopup.mockResolvedValue({ user: { uid: 'google-uid' } })

    const user = await signInWithGoogle()

    expect(user).toEqual({ uid: 'google-uid' })
    expect(mocks.signInWithPopup).toHaveBeenCalledOnce()
    expect(mocks.linkWithPopup).not.toHaveBeenCalled()
  })

  it('links instead of signing in when the current user is anonymous', async () => {
    // Linking upgrades the anonymous account in place, preserving the UID the
    // backend keys sessions by — a guest's history must survive signing in.
    const { signInWithGoogle } = await loadFirebase()
    const anon = { isAnonymous: true }
    mocks.authInstance.currentUser = anon
    mocks.linkWithPopup.mockResolvedValue({ user: { uid: 'same-uid' } })

    const user = await signInWithGoogle()

    expect(user).toEqual({ uid: 'same-uid' })
    expect(mocks.linkWithPopup).toHaveBeenCalledWith(anon, expect.anything())
    expect(mocks.signInWithPopup).not.toHaveBeenCalled()
  })

  it('falls back to plain sign-in when the Google account already exists', async () => {
    const { signInWithGoogle } = await loadFirebase()
    mocks.authInstance.currentUser = { isAnonymous: true }
    mocks.linkWithPopup.mockRejectedValue(firebaseError('auth/credential-already-in-use'))
    mocks.credentialFromError.mockReturnValue({ providerId: 'google.com' })
    mocks.signInWithCredential.mockResolvedValue({ user: { uid: 'existing-uid' } })

    const user = await signInWithGoogle()

    expect(user).toEqual({ uid: 'existing-uid' })
    // Reuses the credential from the failed link — no second popup.
    expect(mocks.signInWithCredential).toHaveBeenCalledOnce()
    expect(mocks.signInWithPopup).not.toHaveBeenCalled()
  })

  it('falls back to redirect only when the popup mechanism itself fails', async () => {
    const { signInWithGoogle } = await loadFirebase()
    mocks.signInWithPopup.mockRejectedValue(firebaseError('auth/popup-blocked'))
    mocks.signInWithRedirect.mockResolvedValue(undefined)

    const user = await signInWithGoogle()

    expect(user).toBeNull()
    expect(mocks.signInWithRedirect).toHaveBeenCalledOnce()
  })

  it('treats a closed popup as a cancel — the next attempt is a popup again', async () => {
    const { signInWithGoogle } = await loadFirebase()
    mocks.signInWithPopup.mockRejectedValueOnce(firebaseError('auth/popup-closed-by-user'))

    await expect(signInWithGoogle()).rejects.toThrow('popup-closed-by-user')

    mocks.signInWithPopup.mockResolvedValueOnce({ user: { uid: 'second-try' } })
    const user = await signInWithGoogle()

    expect(user).toEqual({ uid: 'second-try' })
    expect(mocks.signInWithPopup).toHaveBeenCalledTimes(2)
    // The old latch demoted every attempt after a close to the redirect flow.
    expect(mocks.signInWithRedirect).not.toHaveBeenCalled()
  })
})

describe('getIdToken', () => {
  it('waits for the persisted user to be restored before reading it', async () => {
    // The bug this guards: reading currentUser synchronously on a cold load
    // returned null while IndexedDB was still being read, so the first
    // requests of a signed-in refresh went out unauthenticated and 401ed.
    let fireAuthReady: (() => void) | undefined
    mocks.onAuthStateChanged.mockImplementation((_auth, callback) => {
      fireAuthReady = () => callback(mocks.authInstance.currentUser)
      return () => {}
    })
    const { getIdToken } = await loadFirebase()

    let settled = false
    const pending = getIdToken().then((token) => {
      settled = true
      return token
    })
    await Promise.resolve()
    expect(settled).toBe(false)

    mocks.authInstance.currentUser = {
      isAnonymous: false,
      getIdToken: () => Promise.resolve('token-123'),
    } as never
    fireAuthReady!()

    await expect(pending).resolves.toBe('token-123')
  })
})
