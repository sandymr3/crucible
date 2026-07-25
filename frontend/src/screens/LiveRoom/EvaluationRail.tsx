import { useMemo } from 'react'

import { BandSparkline, Radar, type RadarAxis } from '../../components/charts'
import { Chip, Label, Panel } from '../../components/primitives'
import { asVerdict, type Verdict } from '../../lib/verdict'
import type { LiveTurn } from '../../store/session'
import s from './LiveRoom.module.css'

/**
 * The right rail: what the interview has established so far.
 *
 * Kept on screen permanently rather than revealed at the end — it is what lets
 * a judge watch grading happen without anyone narrating it.
 */

interface EvaluationRailProps {
  turns: LiveTurn[]
  bandTrajectory: number[]
  /** Undropped areas from the interview plan, when a digest exists. */
  plannedAreas: number
}

export function EvaluationRail({ turns, bandTrajectory, plannedAreas }: EvaluationRailProps) {
  const graded = useMemo(() => turns.filter((turn) => turn.evaluation), [turns])
  const closed = turns.filter((turn) => turn.closed).length

  /**
   * The live radar plots the four RUBRIC dimensions, not the report's domain
   * areas: domain scores are computed during finalization and do not exist
   * while the session is running. Same component, different axes.
   */
  const axes = useMemo<RadarAxis[]>(() => {
    if (graded.length === 0) return []
    const totals = { technical: 0, clarity: 0, depth: 0, structure: 0 }
    for (const turn of graded) {
      const scores = turn.evaluation!.scores
      totals.technical += scores.technical_accuracy
      totals.clarity += scores.communication_clarity
      totals.depth += scores.depth
      totals.structure += scores.structure
    }
    const n = graded.length
    return [
      { label: 'Accuracy', value: totals.technical / n },
      { label: 'Depth', value: totals.depth / n },
      { label: 'Structure', value: totals.structure / n },
      { label: 'Clarity', value: totals.clarity / n },
    ]
  }, [graded])

  const concepts = useMemo(() => {
    // Last verdict wins: a concept re-approached from another angle should show
    // where the candidate ended up, not where they started.
    const seen = new Map<string, Verdict>()
    for (const turn of turns) {
      for (const span of turn.evaluation?.spans ?? []) {
        const verdict = asVerdict(span.verdict)
        if (verdict && span.concept) seen.set(span.concept, verdict)
      }
    }
    return [...seen.entries()]
  }, [turns])

  const expected = Math.max(plannedAreas, closed + 1)

  return (
    <>
      <Panel title="Progress" aside={`${closed} / ~${expected}`}>
        <div
          className={s.progressTrack}
          role="progressbar"
          aria-valuenow={closed}
          aria-valuemin={0}
          aria-valuemax={expected}
        >
          <div
            className={s.progressFill}
            style={{ width: `${Math.min(100, (closed / expected) * 100)}%` }}
          />
        </div>
        <Label tone="quiet">
          {graded.length} graded
          {closed > graded.length ? ` · ${closed - graded.length} in flight` : ''}
        </Label>
      </Panel>

      <Panel title="Score matrix">
        <Radar axes={axes} turnsGraded={graded.length} size={210} />
      </Panel>

      <Panel title="Difficulty">
        <BandSparkline trajectory={bandTrajectory} />
      </Panel>

      <Panel title="Delivery">
        {/* Delivery is measured from the ANSWER AUDIO by a worker after the
            turn, and reaches the client only through GET /turns — it is never
            pushed down the socket. Promising it live would be a lie. */}
        <Label tone="quiet">
          Pace and disfluency are measured from your answer audio, and appear in
          the report.
        </Label>
      </Panel>

      <Panel title="Concept heatmap">
        {concepts.length === 0 ? (
          <Label tone="quiet">Concepts appear here as answers are graded</Label>
        ) : (
          <div className={s.conceptGrid}>
            {concepts.map(([concept, verdict]) => (
              <Chip key={concept} tone={verdict} wrap>
                {concept}
              </Chip>
            ))}
          </div>
        )}
      </Panel>
    </>
  )
}
