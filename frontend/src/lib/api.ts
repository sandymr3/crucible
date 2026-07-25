import { getIdToken } from './firebase'
import type { PersonaId } from './persona'
import type {
  Digest,
  DigestResponse,
  MasteryMap,
  Me,
  Mode,
  Pending,
  PersonaCard,
  Report,
  RetestResponse,
  Roadmap,
  Session,
  SessionUsage,
  StudyAnswerResult,
  StudyDepth,
  StudyQuestion,
  Syllabus,
  MasteryStats,
  Turn,
} from './types'

/**
 * The REST client.
 *
 * Every path here is RELATIVE and that is load-bearing. The backend ships no
 * CORS middleware and registers no OPTIONS routes, so a browser on a different
 * origin is blocked on every /v1 call — while the WebSocket keeps working,
 * because handshakes skip CORS. Same-origin is supplied by the Vite dev proxy
 * and, in production, by the Firebase Hosting rewrite. Hardcoding the Cloud Run
 * URL anywhere in here would break both.
 */
const BASE = '/v1'

/**
 * A failure the backend reported, carrying its own error code.
 *
 * The codes matter: `daily_cap_reached` and `already live` need different UI
 * from a generic failure, and `empty_digest` carries a message written to be
 * shown to the user verbatim.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message || code)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }

  /**
   * True when the session does not exist OR belongs to someone else — the
   * backend renders both as 404 so the endpoint cannot be used to discover
   * which session ids exist. Never render this as "deleted".
   */
  get isNotFound(): boolean {
    return this.status === 404
  }

  get isDailyCap(): boolean {
    return this.status === 429
  }

  get isUnauthenticated(): boolean {
    return this.status === 401
  }
}

interface RequestOptions {
  method?: string
  /** JSON body. Omit for bodyless requests. */
  body?: unknown
  /** For multipart uploads, which set their own content type. */
  form?: FormData
  signal?: AbortSignal
  /** Treat 202 as data rather than an error. */
  allowAccepted?: boolean
}

interface RawResponse {
  status: number
  data: unknown
}

async function request(path: string, options: RequestOptions = {}): Promise<RawResponse> {
  const { method = 'GET', body, form, signal, allowAccepted } = options

  const headers: Record<string, string> = {}
  const token = await getIdToken()
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  // A bodyless POST relies on fetch setting Content-Length: 0 itself. Google's
  // frontend rejects a POST without it with an HTTP 411 that never reaches the
  // container, so nothing appears in the logs. Content-Length is a forbidden
  // header name, so it cannot be set by hand — do not "fix" this by trying.
  const response = await fetch(`${BASE}${path}`, {
    method,
    headers,
    signal,
    body: form ?? (body !== undefined ? JSON.stringify(body) : undefined),
  })

  let data: unknown = null
  const text = await response.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (response.status === 202 && allowAccepted) {
    return { status: 202, data }
  }

  if (!response.ok) {
    const payload = (data ?? {}) as { error?: string; message?: string }
    throw new ApiError(
      response.status,
      payload.error ?? `http_${response.status}`,
      payload.message ?? payload.error ?? response.statusText,
    )
  }

  return { status: response.status, data }
}

async function json<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { data } = await request(path, options)
  return data as T
}

/** Wraps the 202-polling contract shared by /report and /roadmap. */
async function pending<T>(path: string): Promise<Pending<T>> {
  const { status, data } = await request(path, { allowAccepted: true })
  if (status === 202) {
    const body = data as { status: 'generating' | 'not_started'; sessionStatus: Session['status'] }
    return { ready: false, status: body.status, sessionStatus: body.sessionStatus }
  }
  return { ready: true, value: data as T }
}

// ── Identity and catalogue ────────────────────────────────────────────────

export const getMe = () => json<Me>('/me')

export const listPersonas = () =>
  json<{ personas: PersonaCard[] }>('/personas').then((r) => r.personas)

// ── Session lifecycle ─────────────────────────────────────────────────────

export interface CreateSessionRequest {
  mode: Mode
  persona?: PersonaId
  topic?: string
  fixtureId?: string
}

/**
 * The backend decodes with DisallowUnknownFields, so a stray property here is
 * a 400 rather than a silently ignored extra. Only send what the mode needs.
 */
export function createSession(req: CreateSessionRequest): Promise<Session> {
  const body: CreateSessionRequest = { mode: req.mode }
  if (req.persona) body.persona = req.persona
  if (req.topic) body.topic = req.topic
  if (req.fixtureId) body.fixtureId = req.fixtureId
  return json<Session>('/sessions', { method: 'POST', body })
}

export const getSession = (id: string) => json<Session>(`/sessions/${id}`)

export const listSessions = () =>
  json<{ sessions: Session[] }>('/sessions').then((r) => r.sessions)

/**
 * Ends the session and queues the report.
 *
 * THE ONLY thing that produces a report. Socket teardown alone leaves the
 * session in `evaluating` forever. Idempotent, so calling it from both an
 * explicit End and an unload handler is safe — they routinely race.
 */
export const endSession = (id: string) =>
  json<{ status: 'ending' | 'already_complete' }>(`/sessions/${id}/end`, { method: 'POST' })

export const getSessionUsage = (id: string) => json<SessionUsage>(`/sessions/${id}/usage`)

// ── Interview configuration ───────────────────────────────────────────────

/** PDF only, 10 MB. The field name must be `file`. */
export function uploadResume(id: string, file: File): Promise<{ gcsUri: string }> {
  const form = new FormData()
  form.append('file', file)
  return json<{ gcsUri: string }>(`/sessions/${id}/resume`, { method: 'POST', form })
}

/** Capped at 20000 BYTES, so non-ASCII reaches the limit sooner. */
export const attachJD = (id: string, text: string) =>
  json<{ status: string }>(`/sessions/${id}/jd`, { method: 'POST', body: { text } })

/** Synchronous and slow — 15-20s. Show a staged progress screen, not a spinner. */
export const buildDigest = (id: string, signal?: AbortSignal) =>
  json<DigestResponse>(`/sessions/${id}/digest`, { method: 'POST', signal })

/** Areas are marked dropped, never removed, so unchecking restores them. */
export const editPlan = (id: string, droppedAreas: string[]) =>
  json<{ remainingAreas: number }>(`/sessions/${id}/plan`, {
    method: 'PATCH',
    body: { droppedAreas },
  })

// ── Post-session ──────────────────────────────────────────────────────────

export const getReport = (id: string) => pending<Report>(`/sessions/${id}/report`)

/** Every turn with its evaluation and delivery embedded — one call renders all. */
export const listTurns = (id: string) =>
  json<{ turns: Turn[] }>(`/sessions/${id}/turns`).then((r) => r.turns)

/** Queued only after the report exists, so it is legitimately later. */
export const getRoadmap = (id: string) => pending<Roadmap>(`/sessions/${id}/roadmap`)

/**
 * Materialises the roadmap's retest plan into a new, pre-configured session
 * inheriting the digest, JD and resume. Consumes a daily allocation.
 */
export const startRetest = (id: string) =>
  json<RetestResponse>(`/sessions/${id}/retest`, { method: 'POST' })

// ── Study mode ────────────────────────────────────────────────────────────

export const buildSyllabus = (id: string, depth?: StudyDepth, syllabusText?: string) =>
  json<{ syllabus: Syllabus; mastery: MasteryStats }>(`/sessions/${id}/syllabus`, {
    method: 'POST',
    body: { ...(depth ? { depth } : {}), ...(syllabusText ? { syllabusText } : {}) },
  })

export const nextDrill = (id: string) => json<StudyQuestion>(`/sessions/${id}/study/next`)

/** Graded synchronously, unlike an interview turn — the result is in the reply. */
export const submitDrill = (id: string, subtopicId: string, question: string, answer: string) =>
  json<StudyAnswerResult>(`/sessions/${id}/study/answer`, {
    method: 'POST',
    body: { subtopicId, question, answer },
  })

export const getMastery = (id: string) => json<MasteryMap>(`/sessions/${id}/mastery`)

// ── Live socket ───────────────────────────────────────────────────────────

/**
 * The interview socket's URL.
 *
 * Same-origin, matching every REST call, so the dev proxy and the Hosting
 * rewrite both carry it. The token travels as a query parameter because
 * browsers cannot set headers on a WebSocket handshake — there is no
 * alternative. It is mitigated by ID tokens being short-lived, and by never
 * logging a full URL. Do not add a logger that prints this.
 */
export function liveSocketUrl(sessionId: string, token: string, voice?: string): string {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = new URL(`${scheme}//${window.location.host}${BASE}/sessions/${sessionId}/live`)
  url.searchParams.set('token', token)
  if (voice) url.searchParams.set('voice', voice)
  return url.toString()
}

// ── Health ────────────────────────────────────────────────────────────────

/** Probe /health, never /healthz: Google's frontend intercepts the latter. */
export async function health(): Promise<{ status: string; version: string }> {
  const response = await fetch('/health')
  return response.json()
}

export type { Digest }
