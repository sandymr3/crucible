import { useCallback, useEffect, useRef } from 'react'

/**
 * Envelope constants.
 *
 * Raw per-chunk RMS is jumpy — at 20ms granularity it strobes, which reads as a
 * glitch rather than as a voice. A fast attack keeps the orb responsive to
 * speech onset while a slow release lets it fall away smoothly.
 */
const ATTACK = 0.45
const RELEASE = 0.08

/**
 * Applied every frame regardless of input. Without it the orb freezes at
 * whatever level the last chunk carried when audio stops — and a frozen orb
 * reads as a crashed app, which is the exact failure the component exists to
 * avoid.
 */
const SILENCE_DECAY = 0.93

/**
 * Speech RMS normalised to 0..1 sits around 0.05-0.30, so it would drive only
 * the bottom third of the visual range. This lifts it to something that
 * actually moves without pinning at full energy.
 */
const DEFAULT_GAIN = 4

interface AmplitudeOptions {
  gain?: number
}

/**
 * Drives the orb's amplitude without going through React.
 *
 * `push` is called once per output audio chunk — fifty times a second — and
 * writes to a CSS custom property on the element directly. Routing that through
 * state would re-render a component tree fifty times a second to move one
 * number, and would compete with the audio worklet for exactly the frames it
 * cannot afford to lose.
 *
 * Returns a ref to attach to the Orb, and the push function for the audio
 * pipeline to call.
 */
export function useAmplitude({ gain = DEFAULT_GAIN }: AmplitudeOptions = {}) {
  const ref = useRef<HTMLDivElement>(null)
  const target = useRef(0)
  const current = useRef(0)

  useEffect(() => {
    let frame = 0
    let running = true

    function tick() {
      if (!running) return

      target.current *= SILENCE_DECAY

      const t = target.current
      const c = current.current
      const next = c + (t - c) * (t > c ? ATTACK : RELEASE)
      current.current = next < 0.002 ? 0 : next

      // Three decimals is well below what a 22px glow or a 9% scale can
      // resolve, and it keeps the property from churning on noise.
      ref.current?.style.setProperty('--orb-energy', current.current.toFixed(3))
      frame = requestAnimationFrame(tick)
    }

    frame = requestAnimationFrame(tick)
    return () => {
      running = false
      cancelAnimationFrame(frame)
    }
  }, [])

  /**
   * Feed one chunk's level, 0..1. Peak-hold: a louder chunk raises the target
   * immediately, and only the per-frame decay brings it back down. Averaging
   * instead would smear the attack and lose the sense of speech starting.
   */
  const push = useCallback(
    (level: number) => {
      if (!Number.isFinite(level)) return
      const scaled = Math.min(1, Math.max(0, level * gain))
      if (scaled > target.current) target.current = scaled
    },
    [gain],
  )

  /** Force a level directly. For development and tests, not the audio path. */
  const setEnergy = useCallback((level: number) => {
    const clamped = Math.min(1, Math.max(0, level))
    target.current = clamped
    current.current = clamped
    ref.current?.style.setProperty('--orb-energy', clamped.toFixed(3))
  }, [])

  return { ref, push, setEnergy }
}
