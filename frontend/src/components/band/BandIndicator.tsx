import { useEffect, useState } from 'react'

import { clampBand } from '../../lib/band'
import { BAND_ANNOUNCE_DELAY_MS, BAND_FLARE_MS } from '../../lib/reveal'
import { Label } from '../primitives/Label'
import s from './BandIndicator.module.css'

interface BandIndicatorProps {
  band: number
  /**
   * Timestamp of the most recent change. A NEW value triggers the flare; the
   * band number alone is not enough, because a demotion back to a band already
   * seen must still announce itself.
   */
  changedAt?: number | null
  /** Font size of the numeral, in px. */
  size?: number
  label?: string
}

/**
 * The difficulty band, and the moment it moves.
 *
 * The width axis is driven entirely by --band-width from the [data-band]
 * blocks, so this component does not animate it — setBand does, along with the
 * whole thermal field. What this owns is the flare at t=120.
 */
export function BandIndicator({ band, changedAt, size = 26, label = 'BAND' }: BandIndicatorProps) {
  const [flaring, setFlaring] = useState(false)

  useEffect(() => {
    if (!changedAt) return

    // Fires with the toast, after the room has already begun moving, so the
    // sequence reads as one event unfolding rather than several at once.
    const start = setTimeout(() => setFlaring(true), BAND_ANNOUNCE_DELAY_MS)
    const stop = setTimeout(() => setFlaring(false), BAND_ANNOUNCE_DELAY_MS + BAND_FLARE_MS)
    return () => {
      clearTimeout(start)
      clearTimeout(stop)
    }
  }, [changedAt])

  return (
    <span
      className={`${s.indicator} ${flaring ? s.flaring : ''}`}
      style={{ ['--band-size' as string]: `${size}px` }}
    >
      {label && <Label tone="quiet">{label}</Label>}
      <span className={s.numeral}>{clampBand(band)}</span>
      {/* Keyed so a second change restarts the animation rather than being
          swallowed because the element never unmounted. */}
      {flaring && <span key={changedAt} className={s.flare} aria-hidden="true" />}
    </span>
  )
}
