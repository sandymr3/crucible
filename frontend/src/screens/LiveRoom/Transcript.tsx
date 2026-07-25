import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowDown } from 'lucide-react'

import { Button } from '../../components/primitives'
import { HeatmapReveal } from '../../components/verdict'
import type { LiveState } from '../../lib/protocol'
import type { LiveTurn } from '../../store/session'
import s from './LiveRoom.module.css'

/** Distance from the bottom, in px, still counted as "following live". */
const PIN_THRESHOLD = 48

function stamp(at: number, start: number | null): string {
  const elapsed = Math.max(0, Math.floor(((start ? at - start : 0) / 1000) | 0))
  const minutes = Math.floor(elapsed / 60)
  return `${minutes}:${String(elapsed % 60).padStart(2, '0')}`
}

interface TranscriptProps {
  turns: LiveTurn[]
  state: LiveState
  startedAt: number | null
}

/**
 * The candidate's own words, and where the heatmap lands.
 *
 * Auto-scrolls to the newest content, but STOPS the moment the user scrolls up
 * — a transcript that drags you back to the bottom while you are reading is
 * infuriating — and offers a way back rather than silently stranding them.
 */
export function Transcript({ turns, state, startedAt }: TranscriptProps) {
  const scroller = useRef<HTMLDivElement>(null)
  const [pinned, setPinned] = useState(true)

  const toBottom = useCallback((behavior: ScrollBehavior = 'smooth') => {
    const el = scroller.current
    if (el) el.scrollTo({ top: el.scrollHeight, behavior })
  }, [])

  useEffect(() => {
    if (pinned) toBottom()
  }, [turns, pinned, toBottom])

  const onScroll = useCallback(() => {
    const el = scroller.current
    if (!el) return
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    setPinned(distance < PIN_THRESHOLD)
  }, [])

  const hasContent = turns.some((turn) => turn.question || turn.answer)

  return (
    <div className={s.transcript}>
      <div
        className={s.scroller}
        ref={scroller}
        onScroll={onScroll}
        // polite, not assertive: new transcript should not interrupt the
        // interviewer mid-sentence for a screen reader user.
        aria-live="polite"
        aria-label="Live transcript"
      >
        {!hasContent ? (
          <p className={s.empty}>
            The interviewer will open. When you are ready to answer, hold the
            microphone open and speak — then tell it you are done.
          </p>
        ) : (
          turns.map((turn) => {
            if (!turn.question && !turn.answer && !turn.interim) return null
            const isLive = !turn.closed
            const evaluating = isLive && state === 'EVALUATING'

            return (
              <article className={s.turn} key={`${turn.index}-${turn.at}`}>
                <span className={s.timestamp}>{stamp(turn.at, startedAt)}</span>
                <div>
                  {turn.question && <p className={s.turnQuestion}>{turn.question}</p>}

                  {turn.spans.length > 0 ? (
                    // Graded. The reveal runs once per evaluation, keyed so an
                    // unrelated re-render cannot replay it.
                    <HeatmapReveal
                      text={turn.answer}
                      ranges={turn.spans}
                      revealKey={`${turn.index}-${turn.evaluation?.gradedAt ?? ''}`}
                    />
                  ) : (
                    turn.answer && (
                      <p className={`${s.answer} ${evaluating ? s.evaluating : ''}`}>
                        {turn.answer}
                      </p>
                    )
                  )}

                  {turn.interim && (
                    <p className={`${s.answer} ${s.interim}`}>
                      {turn.interim}
                      <span className={s.caret} aria-hidden="true" />
                    </p>
                  )}

                  {turn.ungraded && <p className={s.ungraded}>{turn.ungraded}</p>}
                </div>
              </article>
            )
          })
        )}
      </div>

      {!pinned && (
        <div className={s.jumpPill}>
          <Button
            size="compact"
            icon={<ArrowDown size={16} strokeWidth={1.5} />}
            onClick={() => {
              setPinned(true)
              toBottom()
            }}
          >
            Jump to live
          </Button>
        </div>
      )}
    </div>
  )
}
