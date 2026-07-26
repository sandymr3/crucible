import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowRight, BookOpen, Check, RotateCcw } from 'lucide-react'

import { Button, Chip, Label, Panel } from '../../components/primitives'
import * as api from '../../lib/api'
import { ApiError } from '../../lib/api'
import type { Mastery, MasteryStats, StudyAnswerResult, StudyQuestion, Subtopic } from '../../lib/types'
import type { Verdict } from '../../lib/verdict'
import { useAuth } from '../../store/auth'
import s from './Study.module.css'

/**
 * Study Mode: syllabus decomposition, then the four-archetype drill loop.
 *
 * The setup flow lands here with only a topic on the session — this screen
 * owns building the syllabus (slow, one reasoning call), then drives
 * next → answer → result → next. Grading is SYNCHRONOUS, unlike an interview
 * turn: the learner is sitting on a form, so the result is in the reply.
 */

type Phase =
  | { kind: 'loading' }
  | { kind: 'building' }
  | { kind: 'question'; q: StudyQuestion }
  | { kind: 'grading'; q: StudyQuestion }
  | { kind: 'result'; q: StudyQuestion; r: StudyAnswerResult }
  | { kind: 'complete'; mastery: MasteryStats | null }
  | { kind: 'error'; message: string; retryable: boolean }

/** Mastery rendered in the product's own verdict language. */
const MASTERY_TONE: Record<Mastery, Verdict | undefined> = {
  unseen: undefined,
  attempted: 'unsupported',
  shaky: 'incomplete',
  solid: 'validated',
}

const MASTERY_COPY: Record<Mastery, string> = {
  unseen: 'unseen',
  attempted: 'attempted',
  shaky: 'shaky',
  solid: 'solid',
}

function errorPhase(err: unknown): Phase {
  if (err instanceof ApiError) {
    if (err.isDailyCap) return { kind: 'error', message: err.message, retryable: false }
    if (err.isNotFound)
      return { kind: 'error', message: 'This study session does not exist.', retryable: false }
    return { kind: 'error', message: err.message, retryable: true }
  }
  return { kind: 'error', message: 'Something failed. Retry.', retryable: true }
}

export default function Study() {
  const { id } = useParams<{ id: string }>()
  const [phase, setPhase] = useState<Phase>({ kind: 'loading' })
  const [subtopics, setSubtopics] = useState<Subtopic[]>([])
  const [topic, setTopic] = useState('')
  const [answer, setAnswer] = useState('')
  const answerRef = useRef<HTMLTextAreaElement>(null)

  const refreshMastery = useCallback(async () => {
    if (!id) return
    try {
      const map = await api.getMastery(id)
      setSubtopics(map.subtopics)
      setTopic(map.topic)
    } catch {
      // The rail is enrichment; the drill loop carries its own stats.
    }
  }, [id])

  const advance = useCallback(async () => {
    if (!id) return
    try {
      const q = await api.nextDrill(id)
      if (q.complete) {
        setPhase({ kind: 'complete', mastery: q.mastery })
      } else {
        setPhase({ kind: 'question', q })
        setAnswer('')
        requestAnimationFrame(() => answerRef.current?.focus())
      }
      void refreshMastery()
    } catch (err) {
      setPhase(errorPhase(err))
    }
  }, [id, refreshMastery])

  const authLoading = useAuth((state) => state.loading)

  // First load: a session fresh from setup has no syllabus yet — build it,
  // honestly slowly, then enter the loop. One that already has one resumes.
  // Gated on auth restore: fetching before Firebase rehydrates the user sends
  // no Authorization header and 401s a signed-in refresh.
  useEffect(() => {
    if (!id || authLoading) return
    let cancelled = false
    ;(async () => {
      try {
        await api.nextDrill(id).then((q) => {
          if (cancelled) return
          if (q.complete) setPhase({ kind: 'complete', mastery: q.mastery })
          else setPhase({ kind: 'question', q })
          void refreshMastery()
        })
      } catch (err) {
        if (cancelled) return
        if (err instanceof ApiError && err.code === 'no_syllabus') {
          setPhase({ kind: 'building' })
          try {
            const built = await api.buildSyllabus(id)
            if (cancelled) return
            setSubtopics(built.syllabus.subtopics)
            setTopic(built.syllabus.topic)
            await advance()
          } catch (buildErr) {
            if (!cancelled) setPhase(errorPhase(buildErr))
          }
        } else {
          setPhase(errorPhase(err))
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [id, authLoading, advance, refreshMastery])

  async function submit() {
    if (phase.kind !== 'question' || !id) return
    const { q } = phase
    if (!q.subtopicId || !q.question || !answer.trim()) return
    setPhase({ kind: 'grading', q })
    try {
      const r = await api.submitDrill(id, q.subtopicId, q.question, answer.trim())
      setPhase({ kind: 'result', q, r })
      void refreshMastery()
    } catch (err) {
      // The answer is still in the textarea; let them retry the submit.
      setPhase({ kind: 'question', q })
      if (err instanceof ApiError) setPhase(errorPhase(err))
    }
  }

  const stats =
    phase.kind === 'question' || phase.kind === 'grading'
      ? phase.q.mastery
      : phase.kind === 'result'
        ? phase.r.mastery
        : phase.kind === 'complete'
          ? phase.mastery
          : null

  return (
    <main className={s.page}>
      <header className={s.header}>
        <div>
          <Link to="/history" className={s.crumb}>
            ← Your sessions
          </Link>
          <h1 className={s.title}>
            <BookOpen size={26} strokeWidth={1.5} aria-hidden="true" /> {topic || 'Study'}
          </h1>
        </div>
        {stats && (
          <Label tone="quiet">
            {stats.solid} of {stats.total} solid
          </Label>
        )}
      </header>

      <div className={s.grid}>
        <section className={s.main}>
          {phase.kind === 'loading' && <p className={s.notice}>Loading…</p>}

          {phase.kind === 'building' && (
            <Panel title="Decomposing the topic">
              <div className={s.building}>
                <p>
                  Breaking “{topic || 'your topic'}” into a dependency-ordered syllabus — which
                  ideas need which, and in what order to drill them.
                </p>
                <p className={s.buildingNote}>
                  One reasoning call, usually 10–30 seconds. Worth the wait: the order is why
                  the drills feel like they build on each other.
                </p>
                <div className={s.pulseBar} aria-hidden="true" />
              </div>
            </Panel>
          )}

          {(phase.kind === 'question' || phase.kind === 'grading') && (
            <Panel
              title={phase.q.subtopic ?? 'Question'}
              aside={phase.q.archetypeLabel ?? phase.q.archetype}
            >
              <div className={s.drill}>
                <p className={s.question}>{phase.q.question}</p>
                <textarea
                  ref={answerRef}
                  className={s.answer}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                  placeholder={
                    phase.q.archetype === 'teach_back'
                      ? 'Explain it as if to a colleague who has never seen it. Solid is only reachable through this.'
                      : 'Answer in your own words. Graded exactly like a spoken interview answer.'
                  }
                  disabled={phase.kind === 'grading'}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                      e.preventDefault()
                      void submit()
                    }
                  }}
                />
                <div className={s.drillActions}>
                  <span className={s.kbdHint}>Ctrl+Enter</span>
                  <Button
                    variant="primary"
                    onClick={submit}
                    disabled={phase.kind === 'grading' || !answer.trim()}
                  >
                    {phase.kind === 'grading' ? 'Grading…' : 'Submit answer'}
                  </Button>
                </div>
              </div>
            </Panel>
          )}

          {phase.kind === 'result' && (
            <Panel title={phase.q.subtopic ?? 'Result'} aside={phase.r.passed ? 'Passed' : 'Held'}>
              <div className={s.result}>
                <div className={s.resultHead}>
                  <span
                    className={s.resultScore}
                    data-passed={phase.r.passed || undefined}
                  >
                    {phase.r.evaluation.turnScore.toFixed(1)}
                  </span>
                  <div className={s.resultMastery}>
                    <Chip tone={MASTERY_TONE[phase.r.masteryFrom]}>
                      {MASTERY_COPY[phase.r.masteryFrom]}
                    </Chip>
                    <ArrowRight size={16} strokeWidth={1.5} aria-hidden="true" />
                    <Chip tone={MASTERY_TONE[phase.r.masteryTo]}>
                      {MASTERY_COPY[phase.r.masteryTo]}
                    </Chip>
                  </div>
                </div>

                <p className={s.resultSummary}>{phase.r.evaluation.verdict_summary}</p>

                {phase.r.evaluation.concepts_missing.length > 0 && (
                  <p className={s.resultMissing}>
                    Missing: {phase.r.evaluation.concepts_missing.join(' · ')}
                  </p>
                )}

                {phase.r.unlocked && phase.r.unlocked.length > 0 && (
                  <p className={s.unlocked}>
                    <Check size={16} strokeWidth={2} aria-hidden="true" /> Unlocked:{' '}
                    {phase.r.unlocked.join(', ')}
                  </p>
                )}

                <div className={s.drillActions}>
                  <Button
                    variant="primary"
                    icon={<ArrowRight size={20} strokeWidth={1.5} />}
                    onClick={advance}
                  >
                    {phase.r.complete ? 'Finish' : 'Next question'}
                  </Button>
                </div>
              </div>
            </Panel>
          )}

          {phase.kind === 'complete' && (
            <Panel title="Syllabus complete">
              <div className={s.result}>
                <p className={s.resultSummary}>
                  Every subtopic drilled{stats ? ` — ${stats.solid} of ${stats.total} solid` : ''}.
                  Solid was only ever granted through a passed teach-back: reciting is not
                  understanding.
                </p>
                <div className={s.drillActions}>
                  <Link to="/history">
                    <Button variant="primary">Back to your sessions</Button>
                  </Link>
                </div>
              </div>
            </Panel>
          )}

          {phase.kind === 'error' && (
            <Panel title="Stalled">
              <div className={s.result}>
                <p className={s.resultSummary}>{phase.message}</p>
                {phase.retryable && (
                  <div className={s.drillActions}>
                    <Button icon={<RotateCcw size={18} strokeWidth={1.5} />} onClick={advance}>
                      Retry
                    </Button>
                  </div>
                )}
              </div>
            </Panel>
          )}
        </section>

        <aside className={s.rail}>
          <Panel title="Mastery" aside={topic ? undefined : ''}>
            {subtopics.length === 0 ? (
              <p className={s.railEmpty}>The map appears once the syllabus is built.</p>
            ) : (
              <ol className={s.masteryList}>
                {subtopics.map((sub) => (
                  <li key={sub.id} className={s.masteryRow}>
                    <span className={s.masteryName} title={sub.why}>
                      {sub.name}
                    </span>
                    <Chip tone={MASTERY_TONE[sub.mastery]}>{MASTERY_COPY[sub.mastery]}</Chip>
                  </li>
                ))}
              </ol>
            )}
          </Panel>
        </aside>
      </div>
    </main>
  )
}
