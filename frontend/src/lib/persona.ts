/**
 * The three interviewers.
 *
 * Names, taglines and rubric weights are served by GET /v1/personas — the
 * backend owns those. What lives here is the design system's side of it: which
 * stop of the thermal ramp each persona is identified by (design PRD §3.3).
 *
 * The accent tints the orb, the name, and the card border. Nothing else.
 */

export type PersonaId = 'tech_lead' | 'architect' | 'pm'

export const PERSONA_IDS: PersonaId[] = ['tech_lead', 'architect', 'pm']

/** Cold and precise, measured and structural, warmer and more human. */
export const PERSONA_ACCENT: Record<PersonaId, string> = {
  tech_lead: 'var(--persona-tech-lead)',
  architect: 'var(--persona-architect)',
  pm: 'var(--persona-pm)',
}

/** Fallback names, for rendering before /v1/personas has answered. */
export const PERSONA_FALLBACK_NAME: Record<PersonaId, string> = {
  tech_lead: 'The Tech Lead',
  architect: 'The System Architect',
  pm: 'The Product Manager',
}

const KNOWN = new Set<string>(PERSONA_IDS)

export function asPersonaId(value: unknown): PersonaId | null {
  return typeof value === 'string' && KNOWN.has(value) ? (value as PersonaId) : null
}

/** The accent for a persona, falling back to the tech lead's — the demo path. */
export function personaAccent(persona: PersonaId | null | undefined): string {
  return persona ? PERSONA_ACCENT[persona] : PERSONA_ACCENT.tech_lead
}
