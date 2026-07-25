import { useEffect, useRef, useState } from 'react'

/**
 * Fires once when an element crosses into view.
 *
 * Once, deliberately: content that re-animates every time it scrolls past
 * becomes noise, and the home page allows exactly one scroll animation.
 *
 * Falls back to "already visible" where IntersectionObserver is unavailable or
 * motion is reduced, so content is never withheld behind an effect that will
 * not run.
 */
export function useInView<T extends HTMLElement>(threshold = 0.2) {
  const ref = useRef<T>(null)
  const [inView, setInView] = useState(
    () =>
      typeof IntersectionObserver === 'undefined' ||
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true,
  )

  useEffect(() => {
    if (inView) return
    const element = ref.current
    if (!element) return

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setInView(true)
          observer.disconnect()
        }
      },
      { threshold },
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [inView, threshold])

  return { ref, inView }
}
