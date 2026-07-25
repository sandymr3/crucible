/**
 * The four-verdict taxonomy, shared by every surface that renders one.
 *
 * Four values rather than two, because most of what looks like a wrong claim in
 * an interview answer is not falsehood but unbacked assertion. Conflating them
 * produces the failure the backend works hardest to avoid: flagging a true
 * statement red. The backend goes further and rewrites a low-confidence
 * `incorrect` to `unsupported` server-side, so a red span that reaches the
 * client has already cleared a confidence gate.
 */

export type Verdict = 'validated' | 'incomplete' | 'unsupported' | 'incorrect'

export const VERDICTS: Verdict[] = ['validated', 'incomplete', 'unsupported', 'incorrect']

/**
 * Colour is NEVER the only signal (§12). Around 8% of men have some colour
 * vision deficiency and red/green is the worst possible pair, so every verdict
 * carries a glyph everywhere it appears.
 */
export const VERDICT_GLYPH: Record<Verdict, string> = {
  validated: '✓',
  incomplete: '~',
  unsupported: '?',
  incorrect: '!',
}

export const VERDICT_NAME: Record<Verdict, string> = {
  validated: 'Validated',
  incomplete: 'Incomplete',
  unsupported: 'Unsupported',
  incorrect: 'Incorrect',
}

/**
 * Plain-English definitions, shown on the home page's verdict scale so the Live
 * Room needs no explanation later.
 *
 * The `unsupported` wording is load-bearing: it is the one people misread as
 * "the AI thinks I'm lying". Keep it.
 */
export const VERDICT_DEFINITION: Record<Verdict, string> = {
  validated: 'Correct and substantive.',
  incomplete: 'Right as far as it goes. The mechanism is missing.',
  unsupported: 'A specific claim with nothing behind it.',
  incorrect: 'Confidently wrong.',
}

/** The CSS custom property carrying this verdict's colour. */
export function verdictColor(verdict: Verdict): string {
  return `var(--verdict-${verdict})`
}

/** Accessible description for a span, so the heatmap survives a screen reader. */
export function verdictAria(verdict: Verdict, concept: string): string {
  return `${VERDICT_NAME[verdict]}: ${concept}`
}

const KNOWN = new Set<string>(VERDICTS)

/**
 * Narrows an unknown verdict off the wire. The backend validates its own
 * enum and drops unrecognised values, so this only guards against protocol
 * drift — but an unrecognised verdict must not be coloured as if it were a
 * judgement the model actually made.
 */
export function asVerdict(value: unknown): Verdict | null {
  return typeof value === 'string' && KNOWN.has(value) ? (value as Verdict) : null
}
