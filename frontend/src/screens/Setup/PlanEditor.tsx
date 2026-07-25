import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowRight, Check } from 'lucide-react'

import { Button, Label } from '../../components/primitives'
import * as api from '../../lib/api'
import type { Digest, DigestPlanArea } from '../../lib/types'
import s from './Setup.module.css'
import { SetupShell } from './SetupShell'

/**
 * The interview plan, as a checklist.
 *
 * A small feature with an outsized effect: it turns the tool from something
 * happening TO the candidate into something they configured. Areas are marked
 * dropped rather than removed, so unchecking one restores it.
 */
export default function PlanEditor() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [areas, setAreas] = useState<DigestPlanArea[]>([])
  const [dropped, setDropped] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    api
      .getSession(id)
      .then((session) => {
        const plan = (session.digest as Digest | undefined)?.interview_plan ?? []
        setAreas(plan)
        setDropped(new Set(plan.filter((area) => area.dropped).map((area) => area.area)))
      })
      .catch((err) => setError(err instanceof api.ApiError ? err.message : String(err)))
      .finally(() => setLoading(false))
  }, [id])

  const remaining = areas.length - dropped.size

  function toggle(area: string) {
    setDropped((prev) => {
      const next = new Set(prev)
      if (next.has(area)) next.delete(area)
      else next.add(area)
      return next
    })
  }

  async function enter() {
    if (!id) return
    setBusy(true)
    setError(null)
    try {
      // Only worth a call when something actually changed; the backend rejects
      // an empty plan, and sending an unchanged one is a wasted round trip.
      if (dropped.size > 0) await api.editPlan(id, [...dropped])
      navigate(`/room/${id}`)
    } catch (err) {
      setError(err instanceof api.ApiError ? err.message : String(err))
      setBusy(false)
    }
  }

  if (loading) {
    return (
      <SetupShell step="plan" title="Loading your plan…">
        <div className={s.working}>
          <Label tone="quiet">One moment.</Label>
        </div>
      </SetupShell>
    )
  }

  // A retest inherits its digest, and a session without one still has a
  // perfectly good interview ahead of it — the interviewer simply follows the
  // candidate's strongest claims instead of a fixed plan.
  if (areas.length === 0) {
    return (
      <SetupShell step="plan" title="Ready when you are">
        <p className={s.lede}>
          There is no fixed plan for this session, so the interviewer will follow
          whatever you raise.
        </p>
        <div className={s.actions}>
          <span className={s.spacer} />
          <Button
            variant="primary"
            size="hero"
            onClick={() => navigate(`/room/${id}`)}
            icon={<ArrowRight size={20} strokeWidth={1.5} />}
          >
            Enter the room
          </Button>
        </div>
      </SetupShell>
    )
  }

  return (
    <SetupShell
      step="plan"
      title="What should it press you on?"
      lede="Uncheck anything you would rather not spend the ten minutes on. You can keep them all."
    >
      <div className={s.areas}>
        {areas.map((area) => {
          const off = dropped.has(area.area)
          return (
            <button
              key={area.area}
              type="button"
              className={`${s.area} ${off ? s.areaDropped : ''}`}
              onClick={() => toggle(area.area)}
              aria-pressed={!off}
            >
              <span className={`${s.checkbox} ${off ? '' : s.checkboxOn}`}>
                {!off && <Check size={14} strokeWidth={2.5} />}
              </span>
              <span>
                <span className={s.areaName}>{area.area}</span>
                <span className={s.areaWhy}>{area.why}</span>
              </span>
              <Label tone="quiet">Band {area.target_band}</Label>
            </button>
          )
        })}
      </div>

      {remaining === 0 && (
        <p className={s.error}>Keep at least one area — there has to be something to ask about.</p>
      )}
      {error && <p className={s.error}>{error}</p>}

      <div className={s.actions}>
        <Label tone="quiet">
          {remaining} of {areas.length} areas
        </Label>
        <span className={s.spacer} />
        <Button
          variant="primary"
          size="hero"
          onClick={enter}
          disabled={busy || remaining === 0}
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
        >
          {busy ? 'Saving…' : 'Enter the room'}
        </Button>
      </div>
    </SetupShell>
  )
}
