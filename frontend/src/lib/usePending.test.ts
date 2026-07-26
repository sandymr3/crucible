import { createElement } from 'react'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'

import { ApiError } from './api'
import type { Pending } from './types'
import { isSettled, usePending, type PendingStatus } from './usePending'

describe('polling termination', () => {
  it('keeps polling only while generating', () => {
    // Finalization waits on outstanding grades and can legitimately take tens
    // of seconds, so a 202 saying "generating" means keep asking.
    expect(isSettled('generating')).toBe(false)
    expect(isSettled('loading')).toBe(false)
  })

  it('treats not_started as terminal, despite it arriving as a 202', () => {
    // The backend returns not_started and generating with the SAME status
    // code. Polling not_started forever is a spinner that can never resolve,
    // because a session that was never ended has no report coming.
    expect(isSettled('not_started')).toBe(true)
  })

  it('stops on success and on failure', () => {
    expect(isSettled('ready')).toBe(true)
    expect(isSettled('error')).toBe(true)
  })

  it('covers every status, so a new one cannot be silently non-terminal', () => {
    const all: PendingStatus[] = ['loading', 'generating', 'not_started', 'ready', 'error']
    expect(all.filter(isSettled)).toEqual(['not_started', 'ready', 'error'])
  })
})

/**
 * Renders the hook in a bare component — no testing library, in keeping with
 * the rest of the suite. `act` flushes effects and state updates.
 */
;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function renderPending<T>(fetcher: () => Promise<Pending<T>>) {
  let latest: ReturnType<typeof usePending<T>>
  function Probe() {
    latest = usePending<T>(fetcher)
    return null
  }
  const root: Root = createRoot(document.createElement('div'))
  act(() => {
    root.render(createElement(Probe))
  })
  return {
    get current() {
      return latest!
    },
    /** Flushes the promise chain a poll attempt awaits. */
    flush: () => act(async () => {}),
    unmount: () => act(() => root.unmount()),
  }
}

describe('auth failure recovery', () => {
  let harness: ReturnType<typeof renderPending<string>> | null = null

  afterEach(() => {
    harness?.unmount()
    harness = null
  })

  it('flags a 401 as unauthenticated, distinct from other failures', async () => {
    // A 401 during Firebase's session restore is recoverable by signing in,
    // so screens need to tell it apart from a genuinely dead report.
    harness = renderPending<string>(() =>
      Promise.reject(new ApiError(401, 'unauthenticated', 'unauthenticated')),
    )
    await harness.flush()

    expect(harness.current.status).toBe('error')
    expect(harness.current.unauthenticated).toBe(true)
    expect(harness.current.settled).toBe(true)
  })

  it('does not flag non-401 failures', async () => {
    harness = renderPending<string>(() =>
      Promise.reject(new ApiError(404, 'not_found', 'not found')),
    )
    await harness.flush()

    expect(harness.current.status).toBe('error')
    expect(harness.current.unauthenticated).toBe(false)
  })

  it('refetch restarts polling after a terminal error', async () => {
    // The bug this guards: error was terminal with no way back, so the 401
    // from the auth restore race left a permanent "could not be loaded" page
    // even though a retry moments later would have succeeded.
    let failing = true
    harness = renderPending<string>(() =>
      failing
        ? Promise.reject(new ApiError(401, 'unauthenticated', 'unauthenticated'))
        : Promise.resolve({ ready: true, value: 'the report' } as Pending<string>),
    )
    await harness.flush()
    expect(harness.current.status).toBe('error')

    failing = false
    act(() => harness!.current.refetch())
    await harness.flush()

    expect(harness.current.status).toBe('ready')
    expect(harness.current.value).toBe('the report')
    expect(harness.current.unauthenticated).toBe(false)
    expect(harness.current.error).toBeNull()
  })
})
