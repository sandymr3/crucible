import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { CornerDownLeft, Keyboard, Lightbulb, Mic, Square } from 'lucide-react'

import { Orb, useAmplitude } from '../../components/orb'
import { Button, ButtonGroup, Chip, Label, Panel, StatusLabel } from '../../components/primitives'
import { BAND_NAMES, clampBand } from '../../lib/band'
import { PERSONA_FALLBACK_NAME } from '../../lib/persona'
import type { LiveState } from '../../lib/protocol'
import { asVerdict, type Verdict } from '../../lib/verdict'
import { canContinueInText, SESSION_MAX_MS, useSession } from '../../store/session'
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
    if (id) void start(id)
    return () => reset()
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
  const currentTurn = session.turns[session.turns.length - 1]
  const closedTurns = session.turns.filter((turn) => turn.closed).length
  const persona = session.persona ?? 'tech_lead'
  const signature = SIGNATURE[session.state]
  const textStillWorks = canContinueInText(session)

  const concepts = useMemo(() => {
    // Last verdict wins: a concept re-approached from another angle should show
    // where the candidate ended up, not where they started.
    const seen = new Map<string, Verdict>()
    for (const turn of session.turns) {
      for (const span of turn.evaluation?.spans ?? []) {
        const verdict = asVerdict(span.verdict)
        if (verdict && span.concept) seen.set(span.concept, verdict)
      }
    }
    return [...seen.entries()]
  }, [session.turns])

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
          <span className={s.bandIndicator}>
            <Label tone="quiet">BAND</Label>
            <span className={s.bandNumeral}>{clampBand(session.band)}</span>
          </span>
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

          <p className={`${s.question} ${currentTurn?.question ? '' : s.questionEmpty}`}>
            {currentTurn?.question || 'Waiting for the first question…'}
          </p>

          {currentTurn?.hints.map((hint, i) => (
            <div className={s.hintCard} key={i}>
              {hint.text}
              <span className={s.hintPenalty}>−{hint.penalty.toFixed(1)} to this answer</span>
            </div>
          ))}

          <div className={s.controls}>
            <ButtonGroup>
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
                icon={
                  session.micHot ? (
                    <CornerDownLeft size={20} strokeWidth={1.5} />
                  ) : (
                    <Mic size={20} strokeWidth={1.5} />
                  )
                }
                onClick={session.micHot ? session.endAnswer : session.startAnswer}
                disabled={!canAnswer && !session.micHot}
              >
                {session.micHot ? 'I’m done answering' : 'Answer'}
              </Button>
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
                variant="danger"
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
                onClick={() => {
                  session.sendText(draft)
                  setDraft('')
                }}
                disabled={!draft.trim()}
              >
                Send
              </Button>
            </div>
          )}
        </Panel>
      </main>

      <aside className={s.rightRail}>
        <Panel title="Progress">
          <Label tone="loud">
            {closedTurns} answered
          </Label>
        </Panel>

        <Panel title="Score matrix">
          <Label tone="quiet">
            {closedTurns < 3
              ? `Needs ${3 - closedTurns} more answers`
              : 'Charts land in the next step'}
          </Label>
        </Panel>

        <Panel title="Delivery">
          <Label tone="quiet">Measured from your answer audio after the session</Label>
        </Panel>

        <Panel title="Concept heatmap">
          {concepts.length === 0 ? (
            <Label tone="quiet">Concepts appear as answers are graded</Label>
          ) : (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--s-2)' }}>
              {concepts.map(([concept, verdict]) => (
                <Chip key={concept} tone={verdict} wrap>
                  {concept}
                </Chip>
              ))}
            </div>
          )}
        </Panel>
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
