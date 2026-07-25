import { useCallback, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowRight, ExternalLink } from 'lucide-react'

import { Button, Label, Panel } from '../../components/primitives'
import * as api from '../../lib/api'
import { PERSONA_FALLBACK_NAME } from '../../lib/persona'
import type { Roadmap as RoadmapData } from '../../lib/types'
import { usePending } from '../../lib/usePending'
import s from '../Report/Report.module.css'

export default function Roadmap() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const fetchRoadmap = useCallback(() => api.getRoadmap(id!), [id])
  const { value: plan, status, error, settled } = usePending<RoadmapData>(fetchRoadmap, {
    enabled: Boolean(id),
  })

  const [starting, setStarting] = useState(false)
  const [retestError, setRetestError] = useState<string | null>(null)

  async function retest() {
    if (!id) return
    setStarting(true)
    setRetestError(null)
    try {
      // This is the loop closing: a new session inheriting the digest, JD and
      // resume, so nothing is re-uploaded, one band above where they finished.
      const created = await api.startRetest(id)
      navigate(`/setup/${created.sessionId}/persona`)
    } catch (err) {
      setRetestError(
        err instanceof api.ApiError
          ? err.isDailyCap
            ? 'You have used all of today’s sessions. A retest counts as one.'
            : err.message
          : String(err),
      )
      setStarting(false)
    }
  }

  if (!settled || !plan) return <Waiting status={status} error={error} sessionId={id} />

  return (
    <div className={s.page}>
      <header className={s.header}>
        <div>
          <Label tone="quiet">Study plan · {plan.horizon_days} days</Label>
          <h1 className={s.title}>What to close, in order</h1>
        </div>
        <div className={s.headerRight}>
          <Link to={`/report/${id}`}>
            <Button>Back to the report</Button>
          </Link>
        </div>
      </header>

      {plan.summary && <p className={s.waitingBody} style={{ maxWidth: '68ch' }}>{plan.summary}</p>}

      {/* Grounding is not guaranteed. When Search did not fire the plan is
          still useful — the concepts and tasks stand — and saying so is better
          than quietly shipping a plan with no links. */}
      {plan.note && <p className={s.note}>{plan.note}</p>}

      {plan.days.length === 0 ? (
        <Panel title="Nothing to study">
          <Label tone="quiet">
            No significant gaps surfaced in that session. Run a longer interview or
            raise the difficulty to find your edges.
          </Label>
        </Panel>
      ) : (
        <div className={s.days}>
          {plan.days.map((day) => (
            <article className={s.day} key={day.day}>
              <div className={s.dayNumber}>
                <Label tone="quiet">Day</Label>
                <span className={s.dayNumeral}>{day.day}</span>
                <Label tone="quiet">{day.estimated_minutes} min</Label>
              </div>

              <div className={s.dayBody}>
                <h2 className={s.dayConcept}>{day.focus_concept}</h2>
                <p className={s.dayWhy}>{day.why_this_matters}</p>

                {day.resources.length > 0 && (
                  <ul className={s.resources}>
                    {day.resources.map((resource) => (
                      <li className={s.resource} key={resource.url}>
                        <ExternalLink
                          size={14}
                          strokeWidth={1.5}
                          style={{ color: 'var(--dust)', flex: 'none' }}
                        />
                        {/* Every URL was fetched server-side and dead ones were
                            dropped before the plan was stored, so anything here
                            resolves. */}
                        <a
                          className={s.resourceLink}
                          href={resource.url}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          {resource.title || resource.url}
                        </a>
                        <span className={s.resourceMeta}>
                          {resource.type} · {resource.minutes}m
                        </span>
                      </li>
                    ))}
                  </ul>
                )}

                <div className={s.task}>
                  <Label tone="quiet">Do this</Label>
                  <p className={s.taskText}>{day.practice_task}</p>
                  <Label tone="quiet">Then check</Label>
                  <p className={s.taskText}>{day.self_check}</p>
                </div>
              </div>
            </article>
          ))}
        </div>
      )}

      {plan.days.length > 0 && (
        <div className={s.retest}>
          <Label tone="quiet">
            Retest after day {plan.retest_plan.after_day} · band{' '}
            {plan.retest_plan.recommended_band} ·{' '}
            {PERSONA_FALLBACK_NAME[plan.retest_plan.recommended_persona]}
          </Label>
          <p className={s.retestLine}>
            Come back and prove the gap is closed.
          </p>
          {plan.retest_plan.focus_areas.length > 0 && (
            <Label tone="quiet">
              Focused on {plan.retest_plan.focus_areas.join(', ')}
            </Label>
          )}
          <Button
            variant="primary"
            size="hero"
            onClick={retest}
            disabled={starting}
            icon={<ArrowRight size={20} strokeWidth={1.5} />}
          >
            {starting ? 'Setting it up…' : 'Start the retest'}
          </Button>
          {retestError && (
            <Label tone="accent" style={{ color: 'var(--t-assay)' }}>
              {retestError}
            </Label>
          )}
        </div>
      )}
    </div>
  )
}

function Waiting({
  status,
  error,
  sessionId,
}: {
  status: string
  error: string | null
  sessionId?: string
}) {
  if (status === 'error') {
    return (
      <div className={s.waiting}>
        <h1 className={s.waitingTitle}>That plan could not be loaded</h1>
        <p className={s.waitingBody}>{error}</p>
        <Link to="/history">
          <Button>Back to my sessions</Button>
        </Link>
      </div>
    )
  }

  if (status === 'not_started') {
    return (
      <div className={s.waiting}>
        <h1 className={s.waitingTitle}>No plan for this session</h1>
        <p className={s.waitingBody}>
          A study plan is built once an interview has ended and been marked.
        </p>
        {sessionId && (
          <Link to={`/report/${sessionId}`}>
            <Button>See the report</Button>
          </Link>
        )}
      </div>
    )
  }

  return (
    <div className={s.waiting}>
      <div className={s.spinner} />
      <h1 className={s.waitingTitle}>Building your plan</h1>
      {/* Queued only after the report exists, so it is legitimately later —
          and every link is fetched before anyone sees it. */}
      <p className={s.waitingBody}>
        Your gaps are being ordered by what you need first, and every resource link
        is checked before it reaches you. This starts after the report is written.
      </p>
    </div>
  )
}
