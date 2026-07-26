import { useCallback, useEffect, useRef, useState } from 'react'

import { ApiError } from './api'
import type { Pending } from './types'

export type PendingStatus = 'loading' | 'generating' | 'not_started' | 'ready' | 'error'

/**
 * Nothing left to wait for.
 *
 * `generating` is the ONLY status worth another request. `not_started` is
 * terminal even though it is delivered as a 202 alongside `generating` — a
 * session that was never ended has no report coming, and continuing to poll it
 * would be a spinner that never resolves.
 */
export function isSettled(status: PendingStatus): boolean {
  return status === 'ready' || status === 'not_started' || status === 'error'
}

/**
 * The 202-polling contract shared by GET /report and GET /roadmap.
 *
 * Finalization waits on outstanding grades and can legitimately take tens of
 * seconds, so a 202 means "keep asking" rather than "something went wrong".
 *
 * `not_started` is terminal, and that distinction matters: a session that was
 * never ended has no report coming, and continuing to poll would be a lie
 * dressed up as a spinner.
 */
export function usePending<T>(
  fetcher: () => Promise<Pending<T>>,
  { intervalMs = 3000, enabled = true }: { intervalMs?: number; enabled?: boolean } = {},
) {
  const [value, setValue] = useState<T | null>(null)
  const [status, setStatus] = useState<PendingStatus>('loading')
  const [error, setError] = useState<string | null>(null)
  const [unauthenticated, setUnauthenticated] = useState(false)
  const [elapsed, setElapsed] = useState(0)
  // Bumped by refetch() to restart the whole poll loop — a single re-run of
  // load() would stop again after one attempt even if the answer is still
  // `generating`.
  const [attempt, setAttempt] = useState(0)

  // Held in a ref so a caller may pass an inline arrow without restarting the
  // poll on every render.
  const fetchRef = useRef(fetcher)
  fetchRef.current = fetcher

  /** Runs one attempt and reports whether polling should continue. */
  const load = useCallback(async (): Promise<PendingStatus> => {
    try {
      const result = await fetchRef.current()
      if (result.ready) {
        setValue(result.value)
        setStatus('ready')
        setError(null)
        setUnauthenticated(false)
        return 'ready'
      }
      setStatus(result.status)
      return result.status
    } catch (err) {
      setError(err instanceof ApiError || err instanceof Error ? err.message : String(err))
      setUnauthenticated(err instanceof ApiError && err.isUnauthenticated)
      setStatus('error')
      return 'error'
    }
  }, [])

  /** Clears the error and restarts polling from scratch. */
  const refetch = useCallback(() => {
    setStatus('loading')
    setError(null)
    setUnauthenticated(false)
    setAttempt((n) => n + 1)
  }, [])

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>
    const started = Date.now()

    async function tick() {
      if (cancelled) return
      const next = await load()
      if (cancelled) return

      setElapsed(Date.now() - started)
      // Stop the moment there is nothing left to wait for, rather than
      // hammering an endpoint whose answer cannot change.
      if (!isSettled(next)) timer = setTimeout(tick, intervalMs)
    }
    void tick()

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [enabled, intervalMs, load, attempt])

  return { value, status, error, unauthenticated, elapsed, settled: isSettled(status), refetch }
}
