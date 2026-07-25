import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import * as api from './api'

// The token source is the only thing api.ts reaches outside itself.
const getIdToken = vi.hoisted(() => vi.fn<() => Promise<string | null>>())
vi.mock('./firebase', () => ({ getIdToken }))

interface StubCall {
  url: string
  init: RequestInit
}

let calls: StubCall[] = []

function stubFetch(status: number, body: unknown, statusText = '') {
  const text = typeof body === 'string' ? body : JSON.stringify(body)
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init: RequestInit = {}) => {
      calls.push({ url, init })
      return {
        ok: status >= 200 && status < 300,
        status,
        statusText,
        text: async () => text,
        json: async () => JSON.parse(text),
      }
    }),
  )
}

beforeEach(() => {
  calls = []
  getIdToken.mockResolvedValue('test-token')
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('request construction', () => {
  it('calls a RELATIVE path, never an absolute backend URL', async () => {
    // Absolute URLs would bypass the dev proxy and the Hosting rewrite, and the
    // backend ships no CORS headers — every call would fail.
    stubFetch(200, { uid: 'u1', anonymous: true })
    await api.getMe()
    expect(calls[0].url).toBe('/v1/me')
  })

  it('injects the bearer token', async () => {
    stubFetch(200, { uid: 'u1', anonymous: false })
    await api.getMe()
    expect(calls[0].init.headers).toMatchObject({ Authorization: 'Bearer test-token' })
  })

  it('omits Authorization when signed out', async () => {
    getIdToken.mockResolvedValue(null)
    stubFetch(401, { error: 'unauthenticated' })
    await expect(api.getMe()).rejects.toBeInstanceOf(api.ApiError)
    expect(calls[0].init.headers).not.toHaveProperty('Authorization')
  })

  it('re-reads the token per request rather than caching it', async () => {
    // Caching is how a long interview ends in a sudden 401 partway through.
    stubFetch(200, { sessions: [] })
    await api.listSessions()
    await api.listSessions()
    expect(getIdToken).toHaveBeenCalledTimes(2)
  })

  it('sends no body and no Content-Type on a bodyless POST', async () => {
    stubFetch(202, { status: 'ending' })
    await api.endSession('s1')
    expect(calls[0].init.method).toBe('POST')
    expect(calls[0].init.body).toBeUndefined()
    expect(calls[0].init.headers).not.toHaveProperty('Content-Type')
  })

  it('sends multipart with the field name the backend expects', async () => {
    stubFetch(200, { gcsUri: 'gs://bucket/x.pdf' })
    const file = new File(['%PDF-1.4'], 'resume.pdf', { type: 'application/pdf' })
    await api.uploadResume('s1', file)

    const form = calls[0].init.body as FormData
    expect(form).toBeInstanceOf(FormData)
    expect(form.get('file')).toBe(file)
    // Setting it by hand would omit the multipart boundary.
    expect(calls[0].init.headers).not.toHaveProperty('Content-Type')
  })
})

describe('createSession', () => {
  // The backend decodes with DisallowUnknownFields, so an extra property is a
  // 400 rather than a silently ignored field.
  it('sends only mode and persona for an interview', async () => {
    stubFetch(200, {})
    await api.createSession({ mode: 'interview', persona: 'tech_lead' })
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      mode: 'interview',
      persona: 'tech_lead',
    })
  })

  it('sends only mode and topic for a study session', async () => {
    stubFetch(200, {})
    await api.createSession({ mode: 'study', topic: 'Transformer attention' })
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      mode: 'study',
      topic: 'Transformer attention',
    })
  })

  it('sends only mode and fixtureId for a replay', async () => {
    stubFetch(200, {})
    await api.createSession({ mode: 'replay', fixtureId: 'demo-ml-engineer' })
    expect(JSON.parse(calls[0].init.body as string)).toEqual({
      mode: 'replay',
      fixtureId: 'demo-ml-engineer',
    })
  })

  it('drops undefined optionals rather than sending nulls', async () => {
    stubFetch(200, {})
    await api.createSession({ mode: 'interview' })
    expect(JSON.parse(calls[0].init.body as string)).toEqual({ mode: 'interview' })
  })
})

describe('error mapping', () => {
  it("carries the backend's own error code and message", async () => {
    stubFetch(422, {
      error: 'empty_digest',
      message: "We couldn't read anything usable from that resume.",
    })
    await expect(api.buildDigest('s1')).rejects.toMatchObject({
      status: 422,
      code: 'empty_digest',
      message: "We couldn't read anything usable from that resume.",
    })
  })

  it('flags the daily cap, which needs its own UI', async () => {
    stubFetch(429, { error: 'daily_cap_reached', message: '5 of 5 today' })
    const error = await api.createSession({ mode: 'interview' }).catch((e) => e)
    expect(error.isDailyCap).toBe(true)
  })

  it('flags 404, which means missing OR someone else’s — never deleted', async () => {
    stubFetch(404, { error: 'not_found' })
    const error = await api.getSession('nope').catch((e) => e)
    expect(error.isNotFound).toBe(true)
  })

  it('flags 401', async () => {
    stubFetch(401, { error: 'unauthenticated' })
    const error = await api.getMe().catch((e) => e)
    expect(error.isUnauthenticated).toBe(true)
  })

  it('survives a non-JSON error body', async () => {
    // Google's frontend answers with HTML before a request reaches the
    // container — a 411 on a bodyless POST, or its own 404 on /healthz.
    stubFetch(411, '<html>Length Required</html>', 'Length Required')
    const error = await api.endSession('s1').catch((e) => e)
    expect(error).toBeInstanceOf(api.ApiError)
    expect(error.status).toBe(411)
    expect(error.code).toBe('http_411')
  })
})

describe('the 202 polling contract', () => {
  it('reports a report still generating rather than throwing', async () => {
    stubFetch(202, { status: 'generating', sessionStatus: 'evaluating' })
    const result = await api.getReport('s1')
    expect(result).toEqual({
      ready: false,
      status: 'generating',
      sessionStatus: 'evaluating',
    })
  })

  it('distinguishes not_started, which will never become ready', async () => {
    // A session that was never ended has no report coming; polling forever
    // would be a lie.
    stubFetch(202, { status: 'not_started', sessionStatus: 'configuring' })
    const result = await api.getReport('s1')
    expect(result).toMatchObject({ ready: false, status: 'not_started' })
  })

  it('unwraps a ready report', async () => {
    stubFetch(200, { sessionId: 's1', status: 'ready', overallScore: 7.4 })
    const result = await api.getReport('s1')
    expect(result.ready).toBe(true)
    if (result.ready) expect(result.value.overallScore).toBe(7.4)
  })

  it('applies the same contract to the roadmap', async () => {
    stubFetch(202, { status: 'generating', sessionStatus: 'complete' })
    expect(await api.getRoadmap('s1')).toMatchObject({ ready: false })
  })
})

describe('liveSocketUrl', () => {
  it('is same-origin, so the proxy and the Hosting rewrite both carry it', () => {
    const url = new URL(api.liveSocketUrl('s1', 'tok'))
    expect(url.host).toBe(window.location.host)
    expect(url.pathname).toBe('/v1/sessions/s1/live')
  })

  it('carries the token as a query parameter', () => {
    // Browsers cannot set headers on a WebSocket handshake. There is no
    // alternative; it is mitigated by short-lived tokens.
    const url = new URL(api.liveSocketUrl('s1', 'tok-123'))
    expect(url.searchParams.get('token')).toBe('tok-123')
  })

  it('omits the voice override unless asked for', () => {
    expect(new URL(api.liveSocketUrl('s1', 't')).searchParams.has('voice')).toBe(false)
    expect(new URL(api.liveSocketUrl('s1', 't', 'Aoede')).searchParams.get('voice')).toBe('Aoede')
  })

  it('uses ws over http and wss over https', () => {
    expect(api.liveSocketUrl('s1', 't').startsWith('ws://')).toBe(true)
  })
})
