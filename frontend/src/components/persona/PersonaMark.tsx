import { personaAccent, type PersonaId } from '../../lib/persona'

/**
 * An abstract geometric mark per interviewer.
 *
 * Deliberately NOT a face. An illustrated person implies a specific human,
 * which is a claim this product should not make — and a stock photo or a
 * placeholder avatar of a smiling stranger is one of the clearest tells that a
 * page was assembled rather than designed.
 *
 * Each mark is built from the persona's own character so they stay
 * distinguishable at 64px: the Tech Lead is a precise nested grid, the
 * Architect is layered structure, the Product Manager is overlapping circles.
 */
export function PersonaMark({ persona, size = 88 }: { persona: PersonaId; size?: number }) {
  const accent = personaAccent(persona)

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 88 88"
      role="presentation"
      aria-hidden="true"
      style={{ color: accent, display: 'block' }}
    >
      <circle cx="44" cy="44" r="43" fill="var(--vessel-high)" stroke="currentColor" strokeOpacity="0.28" />
      {persona === 'tech_lead' && <TechLead />}
      {persona === 'architect' && <Architect />}
      {persona === 'pm' && <ProductManager />}
    </svg>
  )
}

/** Cold, precise, unsentimental — squares on a strict grid. */
function TechLead() {
  return (
    <g stroke="currentColor" fill="none" strokeWidth="1.5">
      <rect x="26" y="26" width="36" height="36" />
      <rect x="35" y="35" width="18" height="18" strokeOpacity="0.55" />
      <line x1="44" y1="18" x2="44" y2="26" strokeOpacity="0.4" />
      <line x1="44" y1="62" x2="44" y2="70" strokeOpacity="0.4" />
      <line x1="18" y1="44" x2="26" y2="44" strokeOpacity="0.4" />
      <line x1="62" y1="44" x2="70" y2="44" strokeOpacity="0.4" />
    </g>
  )
}

/** Measured, structural — load-bearing layers. */
function Architect() {
  return (
    <g stroke="currentColor" fill="none" strokeWidth="1.5">
      <path d="M22 58 L44 26 L66 58" />
      <line x1="22" y1="58" x2="66" y2="58" />
      <line x1="31" y1="45" x2="57" y2="45" strokeOpacity="0.55" />
      <line x1="38" y1="36" x2="50" y2="36" strokeOpacity="0.35" />
    </g>
  )
}

/** Warmer register — people, overlapping. */
function ProductManager() {
  return (
    <g stroke="currentColor" fill="none" strokeWidth="1.5">
      <circle cx="35" cy="40" r="15" />
      <circle cx="53" cy="40" r="15" strokeOpacity="0.6" />
      <circle cx="44" cy="55" r="15" strokeOpacity="0.35" />
    </g>
  )
}
