import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowRight } from 'lucide-react'

import { Button, Chip, Label, Panel } from '../../components/primitives'
import * as api from '../../lib/api'
import type { Digest } from '../../lib/types'
import s from './Setup.module.css'
import { SetupShell } from './SetupShell'

/**
 * Honest descriptions of what the backend is actually doing, not a fake
 * progress bar.
 *
 * The call is synchronous and measured at 15-20 seconds — the PRD budgeted
 * 4-8 — so this screen has to be genuinely interesting or it reads as broken.
 * Timings are approximate and the copy says what is happening, which is the
 * part that has to be true.
 */
const STAGES = [
  { at: 0, text: 'Reading your resume' },
  { at: 6000, text: 'Finding the claims worth probing' },
  { at: 12000, text: 'Building your interview plan' },
]

export default function DigestReveal() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [digest, setDigest] = useState<Digest | null>(null)
  const [stage, setStage] = useState(0)
  const [error, setError] = useState<{ message: string; recoverable: boolean } | null>(null)
  const started = useRef(false)

  useEffect(() => {
    if (!id || started.current) return
    started.current = true

    async function run() {
      try {
        // Reuse an existing digest rather than re-running the call. A page
        // reload should not cost another model invocation, and the retest path
        // arrives with one already attached.
        const session = await api.getSession(id!)
        if (session.digest && Object.keys(session.digest).length > 0) {
          setDigest(session.digest as Digest)
          return
        }

        const result = await api.buildDigest(id!)
        setDigest(result.digest)
      } catch (err) {
        if (err instanceof api.ApiError) {
          // 422 empty_digest carries a message written to be shown to the user
          // verbatim — almost always a scanned resume. Rewriting it would lose
          // the only actionable advice available.
          setError({ message: err.message, recoverable: err.status === 422 })
        } else {
          setError({ message: String(err), recoverable: false })
        }
      }
    }
    void run()
  }, [id])

  useEffect(() => {
    if (digest || error) return
    const timers = STAGES.map((entry, i) => setTimeout(() => setStage(i), entry.at))
    return () => timers.forEach(clearTimeout)
  }, [digest, error])

  if (error) {
    return (
      <SetupShell step="digest" title="That resume could not be read">
        <p className={s.error}>{error.message}</p>
        <div className={s.actions}>
          <Button onClick={() => navigate(`/setup/${id}`)}>
            {error.recoverable ? 'Try a different file' : 'Back'}
          </Button>
        </div>
      </SetupShell>
    )
  }

  if (!digest) {
    return (
      <SetupShell step="digest" title="Reading what you gave it">
        <div className={s.working}>
          <div className={s.stages}>
            {STAGES.map((entry, i) => (
              <div
                key={entry.text}
                className={`${s.stage} ${
                  i === stage ? s.stageCurrent : i < stage ? s.stagePast : ''
                }`}
              >
                <span className={s.stageMark}>{i < stage ? '✓' : i === stage ? '›' : '·'}</span>
                {entry.text}
              </div>
            ))}
          </div>
          <Label tone="quiet">This takes about twenty seconds.</Label>
        </div>
      </SetupShell>
    )
  }

  const claims = digest.candidate?.claims ?? []
  const role = digest.role
  const gaps = digest.candidate?.gaps_vs_jd ?? []

  return (
    <SetupShell
      step="digest"
      title="Here is what it found"
      lede="These are the claims it will press on, and the questions it has already prepared."
      wide
    >
      {role?.title && (
        <Panel title="The role it is testing you for" aside={role.implied_seniority}>
          <span className={s.claimText}>{role.title}</span>
          {(role.domain_areas?.length ?? 0) > 0 && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--s-2)', marginTop: 'var(--s-4)' }}>
              {role.domain_areas!.map((area) => (
                <Chip key={area}>{area}</Chip>
              ))}
            </div>
          )}
        </Panel>
      )}

      <Panel title="Claims it will probe" aside={`${claims.length}`}>
        <div className={s.claims}>
          {claims.map((claim) => (
            <article className={s.claim} key={claim.id}>
              <p className={s.claimText}>{claim.text}</p>
              <div className={s.claimMeta}>
                <Label tone="quiet">{claim.artifact}</Label>
                <Chip
                  tone={
                    claim.verifiable_depth === 'high'
                      ? 'validated'
                      : claim.verifiable_depth === 'low'
                        ? 'unsupported'
                        : 'incomplete'
                  }
                >
                  {claim.verifiable_depth} depth
                </Chip>
              </div>
              {claim.probe_angles.length > 0 && (
                <ul className={s.probes}>
                  {claim.probe_angles.map((angle) => (
                    <li className={s.probe} key={angle}>
                      {angle}
                    </li>
                  ))}
                </ul>
              )}
            </article>
          ))}
        </div>
      </Panel>

      {gaps.length > 0 && (
        <Panel title="What the job asks for that your resume does not show">
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--s-2)' }}>
            {gaps.map((gap) => (
              <Chip key={gap} tone="incomplete" wrap>
                {gap}
              </Chip>
            ))}
          </div>
        </Panel>
      )}

      <div className={s.actions}>
        <span className={s.spacer} />
        <Button
          variant="primary"
          size="hero"
          onClick={() => navigate(`/setup/${id}/plan`)}
          icon={<ArrowRight size={20} strokeWidth={1.5} />}
        >
          Choose what it presses on
        </Button>
      </div>
    </SetupShell>
  )
}
