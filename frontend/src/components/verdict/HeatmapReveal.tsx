import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { revealDuration, SPAN_STAGGER_MS } from '../../lib/reveal'
import { segmentByRanges, type SpanRange } from '../../lib/segments'
import { Popover } from '../primitives/Popover'
import { VerdictSpan } from './VerdictSpan'
import s from './HeatmapReveal.module.css'

interface HeatmapRevealProps {
  /** The transcript, exactly as the backend recorded it. */
  text: string
  /** Graded spans, already converted to UTF-16 character indices. */
  ranges: SpanRange[]
  /**
   * Changing this restarts the reveal. Use the turn id, so a turn reveals once
   * and does not re-animate on unrelated re-renders.
   */
  revealKey?: string | number
  /** False for already-settled turns, e.g. the report's per-turn accordion. */
  animate?: boolean
}

interface Anchor {
  index: number
  left: number
  top: number
}

/**
 * The heatmap: the candidate's own words, lit up by verdict.
 *
 * The single most screenshot-able frame in the product, and the same component
 * the home page's hero demo uses.
 */
export function HeatmapReveal({ text, ranges, revealKey, animate = true }: HeatmapRevealProps) {
  const segments = useMemo(() => segmentByRanges(text, ranges), [text, ranges])
  const spanCount = segments.filter((seg) => seg.kind === 'span').length

  const [revealing, setRevealing] = useState(false)
  const [anchor, setAnchor] = useState<Anchor | null>(null)
  const surfaceRef = useRef<HTMLDivElement>(null)

  // Restart the reveal when the graded turn changes. The container owns the
  // single timer rather than each span owning its own: when it fires it clears
  // data-reveal, which stops the animation's filled value from overriding the
  // spans' hover state.
  useEffect(() => {
    if (!animate || spanCount === 0) {
      setRevealing(false)
      return
    }
    setRevealing(true)
    const timer = setTimeout(() => setRevealing(false), revealDuration(spanCount) + 60)
    return () => clearTimeout(timer)
  }, [animate, spanCount, revealKey])

  // Hovering a span moves the popover, so the layer is measured rather than
  // nested — an inline element that wraps across lines cannot host an
  // absolutely positioned child reliably.
  const showFor = useCallback((index: number, el: HTMLElement) => {
    const surface = surfaceRef.current
    if (!surface) return

    const rect = el.getClientRects()[0] ?? el.getBoundingClientRect()
    const base = surface.getBoundingClientRect()

    setAnchor({
      index,
      // Anchored left, per §7.2.1, and kept inside the surface so a span near
      // the right edge does not push the popover off it.
      left: Math.min(rect.left - base.left, Math.max(0, surface.clientWidth - 280)),
      top: rect.bottom - base.top + 8,
    })
  }, [])

  const hide = useCallback(() => setAnchor(null), [])

  useEffect(() => {
    if (!anchor) return
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') hide()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [anchor, hide])

  const active =
    anchor === null
      ? null
      : (segments.find((seg) => seg.kind === 'span' && seg.index === anchor.index) ?? null)

  return (
    <div className={s.surface} ref={surfaceRef}>
      {segments.map((segment, i) =>
        segment.kind === 'text' ? (
          <span key={i}>{segment.text}</span>
        ) : (
          <VerdictSpan
            key={i}
            verdict={segment.span.verdict}
            concept={segment.span.concept}
            revealing={revealing}
            delay={segment.index * SPAN_STAGGER_MS}
            onMouseEnter={(e) => showFor(segment.index, e.currentTarget)}
            onMouseLeave={hide}
            onFocus={(e) => showFor(segment.index, e.currentTarget)}
            onBlur={hide}
          >
            {segment.text}
          </VerdictSpan>
        ),
      )}

      {anchor && active?.kind === 'span' && (
        <div className={s.popoverLayer} style={{ left: anchor.left, top: anchor.top }}>
          <Popover
            verdict={active.span.verdict}
            concept={active.span.concept}
            explanation={active.span.explanation}
            correction={active.span.correction}
          />
        </div>
      )}
    </div>
  )
}
