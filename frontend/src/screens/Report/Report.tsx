import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowRight, Route as RouteIcon } from 'lucide-react'

import { BandSparkline, PaceDial, Radar } from '../../components/charts'
import { Button, Label, Panel } from '../../components/primitives'
import * as api from '../../lib/api'
import type { Report as ReportData, SessionUsage, Turn } from '../../lib/types'
import { usePending } from '../../lib/usePending'
import s from './Report.module.css'
import { TurnAccordion } from './TurnAccordion'

const DIMENSIONS = [
  ['technical_accuracy', 'Technical accuracy'],
  ['communication_clarity', 'Communication clarity'],
  ['depth', 'Depth'],
  ['structure', 'Structure'],
] as const

export default function Report() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const fetchReport = useCallback(() => api.getReport(id!), [id])
  const { value: report, status, error, settled } = usePending<ReportData>(fetchReport, {
    enabled: Boolean(id),
  })

  const [turns, setTurns] = useState<Turn[]>([])

  // Fetched separately: the report carries per-turn SUMMARIES, while the
  // accordion needs the full turns with their evaluations and transcripts
  // embedded. One call gets all of them.
  useEffect(() => {
    if (!id || status !== 'ready') return
    api.listTurns(id).then(setTurns).catch(() => setTurns([]))
  }, [id, status])

  if (!settled || !report) {
    return <Waiting status={status} error={error} sessionId={id} />
  }

  const graded = report.turnsGraded

  return (
    <div className={s.page}>
      <header className={s.header}>
        <div>
          <Label tone="quiet">Session report</Label>
          <h1 className={s.title}>How that went</h1>
        </div>
        <div className={s.headerRight}>
          <span className={s.overall}>
            <span className={s.overallValue}>{report.overallScore.toFixed(1)}</span>
            <Label tone="quiet">/ 10 overall</Label>
          </span>
          <Button
            variant="primary"
            icon={<RouteIcon size={20} strokeWidth={1.5} />}
            onClick={() => navigate(`/roadmap/${id}`)}
          >
            See the plan
          </Button>
        </div>
      </header>

      <Panel title="Score matrix" aside={`${graded} graded`}>
        <div className={s.scoreGrid}>
          <Radar
            axes={report.domainScores.map((domain) => ({
              label: domain.domain,
              value: domain.score,
            }))}
            turnsGraded={graded}
            size={260}
          />
          <div className={s.dimensions}>
            {DIMENSIONS.map(([key, label]) => (
              <div className={s.dimension} key={key}>
                <div className={s.dimensionHead}>
                  <Label tone="quiet">{label}</Label>
                  <span className={s.dimensionValue}>
                    {report.aggregateScores[key]}/10
                  </span>
                </div>
                <div className={s.meter}>
                  <div
                    className={s.meterFill}
                    style={{ width: `${(report.aggregateScores[key] / 10) * 100}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      </Panel>

      <div className={s.topGrid}>
        <Panel title="Difficulty" aside={`${report.startBand} → ${report.endBand}`}>
          <BandSparkline trajectory={report.bandTrajectory} height={72} />
          <Label tone="quiet">
            {report.endBand > report.startBand
              ? 'It got harder because you earned it.'
              : report.endBand < report.startBand
                ? 'It eased off to rebuild from the mechanism.'
                : 'The difficulty held steady.'}
          </Label>
        </Panel>

        <Panel title="Delivery">
          <Delivery delivery={report.delivery} />
        </Panel>
      </div>

      <div className={s.lists}>
        <Panel title="You proved">
          {report.strengths.length === 0 ? (
            <Label tone="quiet">Nothing was demonstrated clearly enough to list.</Label>
          ) : (
            <ul className={s.list}>
              {report.strengths.map((item) => (
                <li
                  className={s.listItem}
                  key={item}
                  style={{ ['--list-accent' as string]: 'var(--verdict-validated)' }}
                >
                  <span className={s.listGlyph}>✓</span>
                  {item}
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="You need to close">
          {report.gaps.length === 0 ? (
            <Label tone="quiet">
              No significant gaps surfaced. Raise the difficulty to find your edges.
            </Label>
          ) : (
            /* The backend caps this at five. A list of nineteen weaknesses is
               not actionable, it is discouraging — so nothing here adds more. */
            <ul className={s.list}>
              {report.gaps.map((item) => (
                <li
                  className={s.listItem}
                  key={item}
                  style={{ ['--list-accent' as string]: 'var(--verdict-incomplete)' }}
                >
                  <span className={s.listGlyph}>~</span>
                  {item}
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>

      <Panel title="Answer by answer" aside={`${turns.length} turns`}>
        {turns.length === 0 ? (
          <Label tone="quiet">Loading your answers…</Label>
        ) : (
          <TurnAccordion turns={turns} />
        )}
      </Panel>

      <UsageFooter sessionId={id!} />
    </div>
  )
}

/**
 * The session's metered cost, from the backend's per-call token ledger.
 * One honest line — "here is what this session actually consumed" — because
 * unit economics you can quote beat unit economics you estimate.
 */
function UsageFooter({ sessionId }: { sessionId: string }) {
  const [usage, setUsage] = useState<SessionUsage | null>(null)

  useEffect(() => {
    api.getSessionUsage(sessionId).then(setUsage).catch(() => setUsage(null))
  }, [sessionId])

  if (!usage || usage.cost.totalTokens === 0) return null

  const { cost } = usage
  const audio = cost.promptAudioTokens + cost.responseAudioTokens
  return (
    <p className={s.usageLine}>
      This session consumed {cost.totalTokens.toLocaleString()} tokens across{' '}
      {cost.calls} model calls
      {audio > 0 ? ` — ${audio.toLocaleString()} of them audio, the expensive kind` : ''}.
      Metered per call, not estimated.
    </p>
  )
}

function Delivery({ delivery }: { delivery: ReportData['delivery'] }) {
  if (delivery.turnsWithAudio === 0) {
    return (
      <Label tone="quiet">
        Delivery is measured from spoken answers. This session was typed.
      </Label>
    )
  }

  return (
    <>
      <div className={s.deliveryGrid}>
        <PaceDial wpm={delivery.wpm} band={delivery.paceBand} size={190} />
        <div className={s.deliveryStats}>
          <div className={s.stat}>
            {/* The raw COUNT leads. A per-minute rate extrapolated from a
                fifteen-second answer is arithmetically correct and misleading,
                so the rate is secondary. */}
            <span className={s.statValue}>{delivery.fillerTotal}</span>
            <Label tone="quiet">Fillers</Label>
            <span className={s.statSecondary}>
              {delivery.fillerPerMinute.toFixed(1)} per min
            </span>
          </div>
          <div className={s.stat}>
            <span className={s.statValue}>
              {Math.round(delivery.hesitationScore * 100)}
            </span>
            <Label tone="quiet">Hesitation</Label>
            <span className={s.statSecondary}>0 fluent · 100 hesitant</span>
          </div>
          <div className={s.stat}>
            <span className={s.statValue}>
              {Math.round(delivery.speakingTimeMs / 1000)}s
            </span>
            <Label tone="quiet">Speaking</Label>
            <span className={s.statSecondary}>{delivery.turnsWithAudio} answers</span>
          </div>
        </div>
      </div>

      {delivery.observation && (
        <div className={s.observation}>
          <p className={s.observationText}>{delivery.observation}</p>
          {delivery.drill && <p className={s.drill}>Try this next: {delivery.drill}</p>}
        </div>
      )}
    </>
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
        <h1 className={s.waitingTitle}>That report could not be loaded</h1>
        <p className={s.waitingBody}>{error}</p>
        <Link to="/history">
          <Button>Back to my sessions</Button>
        </Link>
      </div>
    )
  }

  if (status === 'not_started') {
    // Terminal: a session that was never ended has no report coming, so this
    // must not sit spinning forever pretending otherwise.
    return (
      <div className={s.waiting}>
        <h1 className={s.waitingTitle}>This session was never finished</h1>
        <p className={s.waitingBody}>
          A report is generated when an interview ends. This one is still open, or was
          closed before any answer was graded.
        </p>
        {sessionId && (
          <Link to={`/room/${sessionId}`}>
            <Button variant="primary" icon={<ArrowRight size={20} strokeWidth={1.5} />}>
              Go back to the room
            </Button>
          </Link>
        )}
      </div>
    )
  }

  return (
    <div className={s.waiting}>
      <div className={s.spinner} />
      <h1 className={s.waitingTitle}>Marking your answers</h1>
      <p className={s.waitingBody}>
        Every answer is graded sentence by sentence, and the last one is still going.
        This takes a few seconds.
      </p>
    </div>
  )
}
