import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Keyboard, Lightbulb, Mic, Square } from 'lucide-react'

import { BandIndicator } from '../../components/band/BandIndicator'
import { Orb, useAmplitude } from '../../components/orb'
import { Button, ButtonGroup, Label, Panel, StatusLabel } from '../../components/primitives'
import { BAND_NAMES, clampBand } from '../../lib/band'
import { PERSONA_FALLBACK_NAME } from '../../lib/persona'
import type { LiveState } from '../../lib/protocol'
import { canContinueInText, SESSION_MAX_MS, useSession } from '../../store/session'
import { EvaluationRail } from './EvaluationRail'
import { Transcript } from './Transcript'
import s from './LiveRoom.module.css'

/**
 * Visual signature per state (design PRD §8.4).
 *
 * The most common failure in a voice UI is that the user cannot tell whether
 * the system is listening, thinking, or dead — so every state is distinct, and
 * the pulse is reserved for the one state where the microphone is genuinely
 * hot. A pulse that runs in other states stops meaning anything.
 */
const SIGNATURE: Record<LiveState, { label: string; color: string; pulse: boolean }> = {
  CONNECTING: { label: 'Connecting', color: 'var(--dust)', pulse: false },
  ASKING: { label: 'Speaking', color: 'var(--t-cool)', pulse: false },
  LISTENING: { label: 'Listening', color: 'var(--state-live)', pulse: true },
  CLOSING: { label: 'Closing', color: 'var(--t-cool)', pulse: false },
  EVALUATING: { label: 'Evaluating', color: 'var(--state-thinking)', pulse: true },
  SETTLED: { label: 'Settled', color: 'var(--state-live)', pulse: false },
  ERROR: { label: 'Error', color: 'var(--t-assay)', pulse: false },
}

/** No warning frame exists, so the client owns the countdown. */
const WARN_REMAINING_MS = 120_000

function clock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
}

export default function LiveRoom() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const session = useSession()
  const amplitude = useAmplitude({ gain: 3 })
  const [typing, setTyping] = useState(false)
  const [draft, setDraft] = useState('')
  const [confirmEnd, setConfirmEnd] = useState(false)
  const [now, setNow] = useState(Date.now())
  const composerRef = useRef<HTMLTextAreaElement>(null)

  const { start, reset } = session

  useEffect(() => {
    // Deferred by a tick, and cancelled in cleanup. StrictMode dev-mounts
    // effects twice, and two eager connects make the second hit the backend's
    // one-live-session-per-user guardrail (409 "user already has a live
    // session") while the first socket is still tearing down — which presents
    // as "Connection lost" the moment the room opens.
    const connect = window.setTimeout(() => {
      if (id) void start(id)
    }, 0)
    return () => {
      window.clearTimeout(connect)
      reset()
    }
  }, [id, start, reset])

  // The orb follows real output amplitude rather than a decorative loop.
  const { push } = amplitude
  const playbackRms = session.playbackStats?.rms
  useEffect(() => {
    if (playbackRms !== undefined) push(playbackRms)
  }, [playbackRms, push])

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])

  // The relay never speaks unprompted in manual activity mode: it waits for a
  // turn boundary that, at session start, has not happened.
  const begun = useRef(false)
  useEffect(() => {
    if (!begun.current && session.state === 'LISTENING' && session.connection === 'open') {
      begun.current = true
      session.beginInterview()
    }
  }, [session])

  // A tab closed mid-interview must still produce a report — POST /end is the
  // only thing that queues one, and it is idempotent.
  const endSession = session.end
  useEffect(() => {
    function onUnload() {
      if (id) navigator.sendBeacon?.(`/v1/sessions/${id}/end`)
    }
    window.addEventListener('pagehide', onUnload)
    return () => window.removeEventListener('pagehide', onUnload)
  }, [id])

  const canAnswer = session.state === 'LISTENING' || session.state === 'ASKING'

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (!event.ctrlKey && !event.metaKey) return
      // Space-to-talk is deliberately avoided: it collides with scrolling.
      if (event.key === 'Enter') {
        event.preventDefault()
        if (session.micHot) session.endAnswer()
        else if (canAnswer) session.startAnswer()
      }
      if (event.key.toLowerCase() === 'h') {
        event.preventDefault()
        session.requestHint()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [session, canAnswer])

  const elapsed = session.startedAt ? now - session.startedAt : 0
  const remaining = SESSION_MAX_MS - elapsed

  // Recording stopwatch for the mic button. Client-side by design — the
  // boundary the timer describes is the client's own activity_start.
  const recStart = useRef<number | null>(null)
  useEffect(() => {
    recStart.current = session.micHot ? Date.now() : null
  }, [session.micHot])
  const recElapsed = session.micHot && recStart.current ? now - recStart.current : 0
  const currentTurn = session.turns[session.turns.length - 1]
  const closedTurns = session.turns.filter((turn) => turn.closed).length
  const persona = session.persona ?? 'tech_lead'
  const signature = SIGNATURE[session.state]
  const textStillWorks = canContinueInText(session)

  // How many question areas the interview plans to cover, for the progress
  // readout. Undropped areas only — a dropped one will never be asked.
  const plannedAreas = useMemo(() => {
    const plan = session.session?.digest?.interview_plan
    if (!Array.isArray(plan)) return 6
    return plan.filter((area) => !(area as { dropped?: boolean })?.dropped).length || 6
  }, [session.session])

  async function finish() {
    setConfirmEnd(false)
    await endSession()
    if (id) navigate(`/report/${id}`)
  }

  return (
    <div className={s.room}>
      <header className={s.header}>
        <span className={s.wordmark}>
          <span className={s.mark} aria-hidden="true" />
          CRUCIBLE
        </span>
        <span className={s.headerContext}>
          {session.session?.topic ?? 'Interview'} · {PERSONA_FALLBACK_NAME[persona]}
        </span>

        <div className={s.headerRight}>
          <BandIndicator band={session.band} changedAt={session.lastBandChange?.at} />
          <span
            className={`${s.timer} ${remaining < WARN_REMAINING_MS ? s.timerWarn : ''}`}
            title="Time remaining in this session"
          >
            {clock(Math.max(0, remaining))}
          </span>
          <Button variant="danger" size="compact" onClick={() => setConfirmEnd(true)}>
            End
          </Button>
        </div>
      </header>

      <aside className={s.leftRail}>
        <Panel title="Interview session" style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
          <div className={s.identity}>
            <Orb ref={amplitude.ref} state={session.state} persona={persona} size={132} />
            <div>
              <Label tone="loud" className={s.personaName}>
                {PERSONA_FALLBACK_NAME[persona]}
              </Label>
            </div>
            <Label tone="quiet" className={s.focusLine}>
              {signature.label}
            </Label>
          </div>

          {/* Scrolls when a long question or stacked hints overflow a short
              viewport. Without this the controls below were pushed off-panel
              and clipped — the room's page never scrolls, so they were gone. */}
          <div className={s.railScroll}>
            <p className={`${s.question} ${currentTurn?.question ? '' : s.questionEmpty}`}>
              {currentTurn?.question || 'Waiting for the first question…'}
            </p>

            {currentTurn?.hints.map((hint, i) => (
              <div className={s.hintCard} key={i}>
                {hint.text}
                <span className={s.hintPenalty}>−{hint.penalty.toFixed(1)} to this answer</span>
              </div>
            ))}
          </div>

          <div className={s.controls}>
            {/* THE control of the screen. One button, two unmistakable states:
                idle it is the screen's single primary action; recording it is
                red, pulsing, timed, and labelled with what clicking does. */}
            <Button
              variant={session.micHot ? 'danger' : typing ? 'secondary' : 'primary'}
              size="hero"
              block
              className={session.micHot ? s.recordHot : ''}
              icon={
                session.micHot ? (
                  <span className={s.recDot} aria-hidden="true" />
                ) : (
                  <Mic size={22} strokeWidth={1.75} />
                )
              }
              onClick={session.micHot ? session.endAnswer : session.startAnswer}
              disabled={!canAnswer && !session.micHot}
              title="Ctrl+Enter"
            >
              {session.micHot ? (
                <>
                  I’m done — grade my answer
                  <span className={s.recElapsed}>{clock(recElapsed)}</span>
                </>
              ) : (
                'Answer out loud'
              )}
            </Button>
            <p className={s.controlsHint}>
              {session.micHot
                ? 'Recording. Click when you finish speaking — or press Ctrl+Enter.'
                : 'Or answer without a microphone:'}
            </p>

            <ButtonGroup>
              <Button
                variant="ghost"
                icon={<Keyboard size={20} strokeWidth={1.5} />}
                onClick={() => {
                  setTyping((v) => !v)
                  requestAnimationFrame(() => composerRef.current?.focus())
                }}
              >
                Type instead
              </Button>
              <Button
                variant="ghost"
                icon={<Lightbulb size={20} strokeWidth={1.5} />}
                onClick={session.requestHint}
                disabled={!canAnswer}
              >
                Request a hint
              </Button>
              <Button
                variant="ghost"
                icon={<Square size={20} strokeWidth={1.5} />}
                onClick={() => setConfirmEnd(true)}
              >
                End interview
              </Button>
            </ButtonGroup>
          </div>
        </Panel>
      </aside>

      <main className={s.centre}>
        <Panel
          title="Live transcript"
          aside={`Q${closedTurns + 1}`}
          flush
          scroll
          style={{ flex: 1 }}
        >
          <Transcript
            turns={session.turns}
            state={session.state}
            startedAt={session.startedAt}
          />

          {typing && (
            <div className={s.composer}>
              <textarea
                ref={composerRef}
                className={s.composerInput}
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="Type your answer. It is graded exactly like speech."
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                    e.preventDefault()
                    session.sendText(draft)
                    setDraft('')
                  }
                }}
              />
              <Button
                variant="primary"
                title="Ctrl+Enter"
                onClick={() => {
                  session.sendText(draft)
                  setDraft('')
                }}
                disabled={!draft.trim()}
              >
                Send answer
              </Button>
            </div>
          )}
        </Panel>
      </main>

      <aside className={s.rightRail}>
        <EvaluationRail
          turns={session.turns}
          bandTrajectory={session.bandTrajectory}
          plannedAreas={plannedAreas}
        />
      </aside>

      <footer className={s.statusBar}>
        <StatusLabel color={signature.color} pulse={signature.pulse}>
          {signature.label}
        </StatusLabel>
        <Label tone="quiet" className={s.statusItem}>
          Turn {closedTurns + 1}
        </Label>
        <Label tone="quiet" className={s.statusItem}>
          Hints {session.hintsUsed}
        </Label>
        <div className={s.statusRight}>
          {session.usage.totalTokens > 0 && (
            <Label tone="quiet">{session.usage.totalTokens.toLocaleString()} tokens</Label>
          )}
          <Label tone="quiet">
            Band {clampBand(session.band)} — {BAND_NAMES[clampBand(session.band)]}
          </Label>
        </div>
      </footer>

      {confirmEnd && (
        <div className={s.overlay} role="dialog" aria-modal="true" aria-label="End interview">
          <div className={s.dialog}>
            <h2 className={s.dialogTitle}>End the interview?</h2>
            <p className={s.dialogBody}>
              Your report is generated from the answers so far. This cannot be resumed.
            </p>
            <div className={s.dialogActions}>
              <Button onClick={() => setConfirmEnd(false)}>Keep going</Button>
              <Button variant="danger" onClick={finish}>
                End and see my report
              </Button>
            </div>
          </div>
        </div>
      )}

      {session.connectionLost && !confirmEnd && (
        <div className={s.overlay} role="dialog" aria-modal="true" aria-label="Connection lost">
          <div className={`${s.dialog} ${s.fatal}`}>
            <h2 className={s.dialogTitle}>Connection lost</h2>
            {/* There is NO reconnect path: the backend emits resumption handles
                but nothing consumes them. Offering a retry would be a lie, and
                so is offering text mode once the socket has actually closed —
                text_answer needs a live socket like everything else. */}
            <p className={s.dialogBody}>
              {session.error?.message ??
                'The interviewer connection dropped and cannot be resumed.'}{' '}
              {textStillWorks
                ? 'You can finish this answer by typing.'
                : 'Every answer already graded is saved.'}
            </p>
            <div className={s.dialogActions}>
              {textStillWorks && (
                <Button
                  onClick={() => {
                    session.dismissConnectionLost()
                    setTyping(true)
                    requestAnimationFrame(() => composerRef.current?.focus())
                  }}
                >
                  Continue in text mode
                </Button>
              )}
              <Button variant="primary" onClick={finish}>
                End and see my report
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
