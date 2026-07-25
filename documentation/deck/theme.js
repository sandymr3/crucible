// The "Forge" design system — white ground.
//
// Crucible is a vessel for high-heat refining. The deck keeps that identity
// through a single molten accent rather than through a dark background: every
// slide is white, and the colour does the work.
//
// ── CONTRAST RULES, do not break these ──────────────────────────────────────
//   molten  FF6A3D on white ≈ 3.0:1  → large bold display text (19pt+) and fills
//   ember   FFB627 on white ≈ 1.8:1  → FILLS ONLY. Never text, at any size.
//   emberInk B45309 on white ≈ 5.9:1 → use when an amber *text* is needed
//
// The four verdict colours are DATA ONLY — they are the product's own visual
// language for span grading, so using them decoratively would be lying with
// colour. On white they render as the product does: a light tint behind darker
// text, exactly like a real highlight.

const C = {
  page: "FFFFFF", // every slide background
  card: "FAF9F8", // standard panel
  raised: "F7F5F2", // container that holds other panels
  sunken: "EFECE8", // inset block
  hair: "E6E3DF", // hairline border
  hairStrong: "D8D4CE",

  ink: "12141C", // display text
  body: "3A3D4D", // paragraphs
  muted: "6E7285", // secondary text, labels
  faint: "9A9EAE", // captions, placeholder interiors

  molten: "FF6A3D", // the one sharp accent
  ember: "FFB627", // fills only — see rules above
  emberInk: "B45309", // amber that is safe as text
  wave: ["E85A2A", "FFA867"], // title waveform gradient, both visible on white

  // Data only. `fill` is the highlight tint, `ink` the text on it.
  verdict: {
    validated: { fill: "D8F0E4", ink: "1B7A56" },
    incomplete: { fill: "F9EBCF", ink: "8A5D08" },
    unsupported: { fill: "DCEBF7", ink: "24618C" },
    incorrect: { fill: "F7DEDD", ink: "A63A35" },
  },

  ok: "1B7A56", // live-status green, safe as text
  okFill: "EAF6F0",
  okLine: "BFE3D2",
};

const F = {
  display: "Arial", // bold, tight-tracked headings
  body: "Calibri", // paragraphs and labels
  mono: "Consolas", // metrics, model IDs, code, URLs
};

// LAYOUT_WIDE is 13.333 x 7.5in.
const G = {
  W: 13.333,
  H: 7.5,
  M: 0.6, // side margin
  get CW() {
    return this.W - this.M * 2;
  },
};

// pptxgenjs mutates option objects in place, converting values to EMU on first
// use. Sharing one shadow object across two add* calls silently corrupts the
// second. Always build a fresh one.
const shadow = (opts = {}) => ({
  type: "outer",
  color: "9C9890",
  blur: 10,
  offset: 2, // must be >= 0; a negative offset corrupts the file
  angle: 90,
  opacity: 0.16,
  ...opts,
});

// Linear interpolation between two hex colours, for the title waveform.
const mix = (a, b, t) => {
  const p = (h) => [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
  const [ar, ag, ab] = p(a);
  const [br, bg, bb] = p(b);
  const ch = (x, y) =>
    Math.round(x + (y - x) * t)
      .toString(16)
      .padStart(2, "0")
      .toUpperCase();
  return ch(ar, br) + ch(ag, bg) + ch(ab, bb);
};

module.exports = { C, F, G, shadow, mix };
