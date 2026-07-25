/** Small text helpers shared across surfaces. */

/**
 * Truncates to a word budget.
 *
 * The verdict popover has a hard 40-word cap (design PRD §9.4): it floats over
 * the transcript, and an explanation that grows past its box covers the very
 * words it is explaining. Truncating is the specified behaviour — overflowing
 * is not.
 */
export function truncateWords(text: string, max: number): string {
  const words = text.trim().split(/\s+/)
  if (words.length <= max) return text.trim()
  return words.slice(0, max).join(' ').replace(/[,;:.]$/, '') + '…'
}
