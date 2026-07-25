import { useMemo, useState } from 'react'
import { ChevronDown } from 'lucide-react'

import { Label } from '../../components/primitives'
import { HeatmapReveal } from '../../components/verdict'
import { resolveSpanRanges, type ByteSpan } from '../../lib/byteOffset'
import type { Turn } from '../../lib/types'
import { VERDICTS, VERDICT_GLYPH, verdictColor } from '../../lib/verdict'
import s from './Report.module.css'

const DIMENSIONS = [
  ['technical_accuracy', 'Accuracy'],
  ['communication_clarity', 'Clarity'],
  ['depth', 'Depth'],
  ['structure', 'Structure'],
] as const

/**
 * One turn, expanded: the question, the answer with its heatmap intact, the
 * four scores, the verdict summary, and what a full-marks answer looks like.
 *
 * The heatmap is the same component the Live Room uses, but with the reveal
 * switched off — the animation is for the moment grading lands, and replaying
 * it every time an accordion opens would make it decoration.
 */
export function TurnAccordion({ turns }: { turns: Turn[] }) {
  const [open, setOpen] = useState<string | null>(turns[0]?.id ?? null)

  return (
    <div className={s.turns}>
      {turns.map((turn) => (
        <TurnRow
          key={turn.id}
          turn={turn}
          open={open === turn.id}
          onToggle={() => setOpen(open === turn.id ? null : turn.id)}
        />
      ))}
    </div>
  )
}

function TurnRow({
  turn,
  open,
  onToggle,
}: {
  turn: Turn
  open: boolean
  onToggle: () => void
}) {
  const evaluation = turn.evaluation

  // Byte offsets to character indices, exactly as in the live path.
  const ranges = useMemo(
    () =>
      evaluation
        ? resolveSpanRanges(turn.userTranscript, evaluation.spans as unknown as ByteSpan[])
        : [],
    [evaluation, turn.userTranscript],
  )

  const counts = useMemo(() => {
    const tally = new Map<string, number>()
    for (const span of evaluation?.spans ?? []) {
      tally.set(span.verdict, (tally.get(span.verdict) ?? 0) + 1)
    }
    return tally
  }, [evaluation])

  return (
    <section className={s.turn}>
      <button className={s.turnHead} onClick={onToggle} aria-expanded={open}>
        <span className={s.turnIndex}>Q{turn.index + 1}</span>
        <span className={s.turnQuestion}>{turn.questionText || 'Question not captured'}</span>

        <span className={s.turnMeta}>
          {VERDICTS.filter((verdict) => counts.get(verdict)).map((verdict) => (
            <span
              key={verdict}
              className={s.spanCount}
              style={{ ['--count-accent' as string]: verdictColor(verdict) }}
              title={verdict}
            >
              {VERDICT_GLYPH[verdict]} {counts.get(verdict)}
            </span>
          ))}
          {evaluation ? (
            <span className={s.turnScore}>{evaluation.turnScore.toFixed(1)}</span>
          ) : (
            <Label tone="quiet">not graded</Label>
          )}
          <ChevronDown
            size={18}
            strokeWidth={1.5}
            style={{
              transform: open ? 'rotate(180deg)' : 'none',
              transition: 'transform var(--dur-fast) var(--ease-out)',
              color: 'var(--dust)',
            }}
          />
        </span>
      </button>

      {open && (
        <div className={s.turnBody}>
          {turn.userTranscript ? (
            <HeatmapReveal
              text={turn.userTranscript}
              ranges={ranges}
              // Settled already: the reveal belongs to the moment grading
              // landed, not to opening a panel afterwards.
              animate={false}
            />
          ) : (
            <Label tone="quiet">No transcript was captured for this answer.</Label>
          )}

          {turn.gradingStatus === 'skipped' && (
            <Label tone="quiet">Too short to grade, so it was not scored.</Label>
          )}
          {turn.gradingStatus === 'failed' && (
            <Label tone="quiet">
              {turn.gradingError ?? 'This answer could not be graded.'}
            </Label>
          )}

          {evaluation && (
            <>
              <div className={s.scoreRow}>
                {DIMENSIONS.map(([key, label]) => (
                  <div className={s.scorePill} key={key}>
                    <Label tone="quiet">{label}</Label>
                    <span className={s.turnScore}>{evaluation.scores[key]}/10</span>
                  </div>
                ))}
                {turn.hintsUsed > 0 && (
                  <div className={s.scorePill}>
                    <Label tone="quiet">Hints</Label>
                    <span className={s.turnScore}>
                      {turn.hintsUsed} · −{(turn.hintsUsed * 0.5).toFixed(1)}
                    </span>
                  </div>
                )}
              </div>

              <p className={s.summary}>{evaluation.verdict_summary}</p>

              {evaluation.ideal_answer_outline.length > 0 && (
                <div>
                  <Label tone="quiet">What a full answer covers</Label>
                  <ul className={s.outline}>
                    {evaluation.ideal_answer_outline.map((line, i) => (
                      <li key={i}>{line}</li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </section>
  )
}
