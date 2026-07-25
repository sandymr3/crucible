// The "Forge" design system.
//
// Crucible is a vessel for high-heat refining, and the deck should look like one
// rather than like generic hackathon blue. Near-black dominates; a single molten
// accent does the pointing; ember carries the numbers.
//
// The four verdict colours are DATA ONLY. They are the product's own visual
// language for span grading, so using them decoratively would be lying with
// colour.

const C = {
  ink: "0E0F14", // dominant dark background
  panel: "1A1C26", // raised card on dark
  panelUp: "24273A", // hover/second-level card on dark
  bone: "F5F3F0", // light slide background
  boneCard: "FFFFFF", // card on light
  slate: "3A3D4D", // body text on light
  slateMute: "6E7285", // secondary text, both grounds
  line: "2C3040", // hairline on dark
  lineLight: "DFDBD6", // hairline on light

  molten: "FF6A3D", // the one sharp accent
  ember: "FFB627", // stat numerals, highlights

  // Data-only. Never decorative.
  validated: "2FB380",
  incomplete: "E0A63C",
  unsupported: "4A9DD4",
  incorrect: "D9534F",
};

const F = {
  display: "Arial", // bold, tight-tracked headings
  body: "Calibri", // paragraphs and labels
  mono: "Consolas", // metrics, model IDs, code, URLs
};

// Slide geometry. LAYOUT_WIDE is 13.333 x 7.5in.
const G = {
  W: 13.333,
  H: 7.5,
  M: 0.6, // side margin (plan floor is 0.55)
  get CW() {
    return this.W - this.M * 2;
  }, // 12.133 content width
};

// pptxgenjs mutates option objects in place, converting values to EMU on first
// use. Sharing one shadow object across two add* calls silently corrupts the
// second. Always build a fresh one.
const shadow = (opts = {}) => ({
  type: "outer",
  color: "000000",
  blur: 12,
  offset: 3, // must be >= 0; a negative offset corrupts the file
  angle: 90,
  opacity: 0.28,
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
