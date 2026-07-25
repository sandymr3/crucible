/**
 * The live session's vocabulary, as the relay emits it.
 *
 * The most common failure in a voice UI is that the user cannot tell whether
 * the system is listening, thinking, or dead — so the backend surfaces every
 * one of these explicitly rather than leaving the client to infer it, and every
 * state must have a distinct visual signature.
 */

export type LiveState =
  | 'CONNECTING'
  | 'ASKING'
  | 'LISTENING'
  | 'CLOSING'
  | 'EVALUATING'
  | 'SETTLED'
  | 'ERROR'

/**
 * Two of these never arrive over the wire, and it matters for anything that
 * switches exhaustively on state:
 *
 *   ERROR    is declared by the backend but never emitted. Failures arrive as a
 *            separate `error` frame and the state does not change, so the
 *            client sets this itself.
 *   SETTLED  is emitted only at the end of a REPLAY session. A live interview
 *            never sends it.
 */
export const CLIENT_ONLY_STATES: LiveState[] = ['ERROR']

/** The microphone is hot and the client may send audio. */
export function isListening(state: LiveState): boolean {
  return state === 'LISTENING'
}

/** The interviewer is speaking, or about to. */
export function isSpeaking(state: LiveState): boolean {
  return state === 'ASKING'
}
