import { describe, expect, it } from 'vitest'

import { isSettled, type PendingStatus } from './usePending'

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
