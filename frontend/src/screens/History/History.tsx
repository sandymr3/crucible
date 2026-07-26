import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowRight, BookOpen, Mic, Play } from 'lucide-react'

import { Button, Chip, Label } from '../../components/primitives'
import * as api from '../../lib/api'
import { ApiError } from '../../lib/api'
import { PERSONA_FALLBACK_NAME } from '../../lib/persona'
import type { Session, SessionStatus } from '../../lib/types'
import { useAuth } from '../../store/auth'
import s from './History.module.css'

/**
 * Every session the caller has, newest first, each linking to wherever that
 * session can actually go next. The status decides the destination — sending
 * a `configuring` session to its report would 202 forever, and a `complete`
 * one back to the room would show an empty socket.
 */

const MODE_ICON = {
  interview: <Mic size={18} strokeWidth={1.5} />,
  study: <BookOpen size={18} strokeWidth={1.5} />,
  replay: <Play size={18} strokeWidth={1.5} />,
} as const

const STATUS_COPY: Record<SessionStatus, string> = {
  configuring: 'Setting up',
  live: 'In progress',
  evaluating: 'Grading',
  complete: 'Complete',
  abandoned: 'Abandoned',
}

/** Where clicking the card takes you. */
function destination(session: Session): { to: string; label: string } {
  if (session.mode === 'study') {
    return { to: `/study/${session.id}`, label: 'Continue studying' }
  }
  switch (session.status) {
    case 'configuring':
      return { to: `/setup/${session.id}`, label: 'Finish setup' }
    case 'live':
      return { to: `/room/${session.id}`, label: 'Return to the room' }
    case 'evaluating':
    case 'complete':
    case 'abandoned':
      return { to: `/report/${session.id}`, label: 'See the report' }
  }
}

function when(iso: string): string {
  const date = new Date(iso)
  const today = new Date()
  const sameDay = date.toDateString() === today.toDateString()
  const time = date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  if (sameDay) return `Today · ${time}`
  return `${date.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} · ${time}`
}

export default function History() {
  const navigate = useNavigate()
  const auth = useAuth()
  const [sessions, setSessions] = useState<Session[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Waits for Firebase to finish restoring the user: fetching immediately on
  // mount races the restore, sends no Authorization header, and a hard
  // refresh reads as signed-out for someone who is signed in.
  useEffect(() => {
    if (auth.loading) return
    if (!auth.user) {
      setError('Sign in to see your sessions.')
      return
    }
    let cancelled = false
    api
      .listSessions()
      .then((list) => {
        if (!cancelled) setSessions(list)
      })
      .catch((err) => {
        if (cancelled) return
        setError(
          err instanceof ApiError && err.isUnauthenticated
            ? 'Sign in to see your sessions.'
            : 'Could not load your sessions. Refresh to retry.',
        )
      })
    return () => {
      cancelled = true
    }
  }, [auth.loading, auth.user])

  return (
    <main className={s.page}>
      <header className={s.header}>
        <div>
          <Link to="/" className={s.crumb}>
            ← Crucible
          </Link>
          <h1 className={s.title}>Your sessions</h1>
        </div>
        <Button
          variant="primary"
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
          onClick={() => navigate('/setup')}
        >
          New session
        </Button>
      </header>

      {error && <p className={s.notice}>{error}</p>}
      {!error && sessions === null && <p className={s.notice}>Loading…</p>}

      {sessions !== null && sessions.length === 0 && (
        <div className={s.empty}>
          <p>
            Nothing yet. Your interviews and study runs land here, each with its report and
            roadmap.
          </p>
        </div>
      )}

      <ul className={s.list}>
        {sessions?.map((session) => {
          const dest = destination(session)
          const name =
            session.topic ||
            (session.persona ? PERSONA_FALLBACK_NAME[session.persona] : 'Interview')
          return (
            <li key={session.id}>
              <Link to={dest.to} className={s.card}>
                <span className={s.modeIcon} data-mode={session.mode} aria-hidden="true">
                  {MODE_ICON[session.mode]}
                </span>

                <span className={s.cardBody}>
                  <span className={s.cardName}>{name}</span>
                  <span className={s.cardMeta}>
                    {when(session.createdAt)}
                    {session.turnCount > 0 && <> · {session.turnCount} turns</>}
                    {session.mode !== 'study' && <> · band {session.difficultyBand}</>}
                  </span>
                </span>

                <Chip>{STATUS_COPY[session.status]}</Chip>
                <span className={s.cardAction}>
                  {dest.label} <ArrowRight size={16} strokeWidth={1.5} aria-hidden="true" />
                </span>
              </Link>

              {/* The roadmap exists only after finalization; offering it any
                  earlier is a guaranteed 202 spinner. */}
              {session.status === 'complete' && session.mode !== 'study' && (
                <div className={s.cardExtras}>
                  <Link to={`/roadmap/${session.id}`}>
                    <Label tone="quiet">Roadmap →</Label>
                  </Link>
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </main>
  )
}
