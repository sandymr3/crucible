import { useEffect, useState } from 'react'

import { Label } from '../../components/primitives'
import { HeatmapReveal } from '../../components/verdict'
import { DEMO_SPANS, DEMO_TRANSCRIPT } from '../../lib/fixtures'
import { SPAN_STAGGER_MS } from '../../lib/reveal'
import s from './Home.module.css'

/**
 * The hero demo (design PRD §7.2.1).
 *
 * The hero does not describe the grading — it PERFORMS it. A visitor
 * understands the whole value proposition in three seconds without scrolling,
 * reading, or signing in.
 *
 * The content is hard-coded and this component never calls the backend: it must
 * render identically every time and must never wait on a network request. It
 * also uses the REAL VerdictSpan and the real reveal timing, because building a
 * second version is how the home page starts lying about the product.
 */

/** The scripted beats, in milliseconds from mount. */
const HOLD_PLAIN_MS = 1400
const POPOVER_AT = 2100
const GRADED_LABEL_AT = 2600
const LOOP_AT = 8000
const RESTART_PAUSE_MS = 800

/** The span the popover explains — the incorrect one, which earns the look. */
const PINNED = 1

type Phase = 'grading' | 'graded'

export function HeroDemo() {
  const [cycle, setCycle] = useState(0)
  const [phase, setPhase] = useState<Phase>('grading')
  const [popover, setPopover] = useState(false)

  const reduced =
    typeof window !== 'undefined' &&
    window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true

  useEffect(() => {
    if (reduced) {
      // Render the end state immediately and never loop. The finding still
      // lands; it simply does not travel.
      setPhase('graded')
      setPopover(true)
      return
    }

    setPhase('grading')
    setPopover(false)

    const timers = [
      setTimeout(() => setPopover(true), POPOVER_AT),
      setTimeout(() => setPhase('graded'), GRADED_LABEL_AT),
      setTimeout(() => {
        setPopover(false)
        setPhase('grading')
      }, LOOP_AT),
      setTimeout(() => setCycle((n) => n + 1), LOOP_AT + RESTART_PAUSE_MS),
    ]
    return () => timers.forEach(clearTimeout)
  }, [cycle, reduced])

  return (
    <div className={s.demoCard}>
      <div className={s.demoHeader}>
        <span className={s.demoDot} data-idle={phase === 'graded' || undefined} />
        <Label tone="quiet">
          {phase === 'grading'
            ? 'Live transcript · grading'
            : `Graded · ${DEMO_SPANS.length} findings`}
        </Label>
      </div>

      <div className={s.demoBody}>
        <HeatmapReveal
          // Restarting the key replays the identical sequence the Live Room
          // runs, rather than a separate approximation of it.
          key={cycle}
          text={DEMO_TRANSCRIPT}
          ranges={DEMO_SPANS}
          revealKey={cycle}
          animate={!reduced}
          startDelayMs={reduced ? 0 : HOLD_PLAIN_MS}
          pinnedSpan={popover ? PINNED : null}
        />
      </div>
    </div>
  )
}

/** Where the spans land, for the caption beneath the card. */
export const HERO_SPAN_STAGGER = SPAN_STAGGER_MS
