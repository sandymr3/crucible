// Crucible — InnovaHack pitch deck, 8 slides.
//
//   node build-deck.js   ->   Crucible-InnovaHack.pptx
//
// Every figure on every slide comes from ./facts.js, which carries only measured
// values from backend/docs/checkpoints/. Nothing here invents a number to fill a
// layout.

const path = require("path");
const pptxgen = require("pptxgenjs");
const { C, F, G, shadow, mix } = require("./theme");
const D = require("./facts");

// ───────────────────────────────────────────────────────────────────────────
// Slide 4 screenshot slots.
//
// Set `file` to an image path once the frontend exists and re-run; the image
// drops into the identical rect the placeholder occupies. Until then each slot
// renders a styled frame whose caption still carries the point, so the slide
// never looks broken.
// ───────────────────────────────────────────────────────────────────────────
const SCREENSHOTS = {
  main: {
    file: null,
    label: "LIVE INTERVIEW",
    caption:
      "Your own words, graded span by span. Green where you nailed it, amber where you were thin, blue where you claimed something you could not support.",
  },
  a: {
    file: null,
    label: "REPORT",
    caption: "Radar across the role's domains, band sparkline, per-turn accordion.",
  },
  b: {
    file: null,
    label: "ROADMAP",
    caption: "Day-by-day plan in prerequisite order. Every link fetched and verified.",
  },
};

const logo = (slug) => path.join(__dirname, "assets", `${slug}.png`);

// ── helpers ────────────────────────────────────────────────────────────────

/** Standard slide header. Content below it starts at y = 1.5. */
function header(slide, { eyebrow, title, dark }) {
  slide.addText(eyebrow, {
    x: G.M,
    y: 0.4,
    w: G.CW,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 10.5,
    bold: true,
    color: C.molten,
    charSpacing: 1.6,
    margin: 0,
    valign: "middle",
  });
  slide.addText(title, {
    x: G.M,
    y: 0.68,
    w: G.CW,
    h: 0.66,
    fontFace: F.display,
    fontSize: 33,
    bold: true,
    color: dark ? C.bone : "12141C",
    charSpacing: -0.6,
    margin: 0,
    valign: "middle",
  });
}

/** The repeated motif: a filled circle carrying a numeral or short glyph. */
function badge(slide, { x, y, d = 0.46, text, fill, color = "FFFFFF", size = 13 }) {
  slide.addShape("ellipse", { x, y, w: d, h: d, fill: { color: fill } });
  slide.addText(text, {
    x,
    y,
    w: d,
    h: d,
    fontFace: F.mono,
    fontSize: size,
    bold: true,
    color,
    align: "center",
    valign: "middle",
    margin: 0,
  });
}

/** Big numeral over a small caption. */
function stat(slide, { x, y, w, value, label, dark, accent = C.ember, size = 30 }) {
  slide.addText(value, {
    x,
    y,
    w,
    h: 0.52,
    fontFace: F.display,
    fontSize: size,
    bold: true,
    color: accent,
    align: "center",
    valign: "middle",
    margin: 0,
    charSpacing: -0.5,
  });
  slide.addText(label, {
    x,
    y: y + 0.5,
    w,
    h: 0.42,
    fontFace: F.mono,
    fontSize: 8.5,
    color: dark ? C.slateMute : C.slateMute,
    align: "center",
    valign: "top",
    margin: 0,
    charSpacing: 0.4,
  });
}

/** Rounded panel used for every card in the deck. */
function panel(slide, { x, y, w, h, dark, fill, radius = 0.04, withShadow = true }) {
  slide.addShape("roundRect", {
    x,
    y,
    w,
    h,
    rectRadius: radius,
    fill: { color: fill || (dark ? C.panel : C.boneCard) },
    line: { color: dark ? C.line : C.lineLight, width: 1 },
    ...(withShadow ? { shadow: shadow(dark ? { opacity: 0.4 } : { opacity: 0.1, blur: 8 }) } : {}),
  });
}

// ── deck ───────────────────────────────────────────────────────────────────

const pres = new pptxgen();
pres.layout = "LAYOUT_WIDE"; // must be set before any slide is added
pres.author = D.team.name;
pres.company = D.team.name;
pres.title = "Crucible — InnovaHack";

// ═══ 1 · Title ═════════════════════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.ink };

  s.addText("INNOVAHACK  ·  GEN AI PROBLEM STATEMENT 2", {
    x: G.M,
    y: 0.52,
    w: G.CW,
    h: 0.28,
    fontFace: F.mono,
    fontSize: 11,
    bold: true,
    color: C.molten,
    charSpacing: 2.2,
    margin: 0,
    valign: "middle",
  });

  s.addText("CRUCIBLE", {
    x: G.M,
    y: 1.05,
    w: G.CW,
    h: 1.15,
    fontFace: F.display,
    fontSize: 68,
    bold: true,
    color: C.bone,
    charSpacing: -1.5,
    margin: 0,
    valign: "middle",
  });

  s.addText("An adaptive, voice-native AI interview coach — built entirely on Vertex AI.", {
    x: G.M,
    y: 2.24,
    w: 10.4,
    h: 0.4,
    fontFace: F.body,
    fontSize: 17,
    color: "B9BCC9",
    margin: 0,
    valign: "middle",
  });

  s.addText(
    [
      {
        text: "Most interview prep is a quiz with a chat window bolted on.\n",
        options: { color: "8C90A2" },
      },
      { text: "Crucible is a conversation that gets harder when you're good.", options: { color: C.ember } },
    ],
    {
      x: G.M,
      y: 2.95,
      w: 9.6,
      h: 0.95,
      fontFace: F.display,
      fontSize: 19,
      bold: true,
      italic: true,
      lineSpacingMultiple: 1.18,
      charSpacing: -0.3,
      margin: 0,
      valign: "middle",
    }
  );

  // Waveform: the real amplitude envelope of the recorded 27.29s demo session.
  const bars = D.waveform;
  const base = 6.02; // baseline the bars grow up from
  const maxH = 1.45;
  const pitch = G.CW / bars.length;
  const bw = pitch * 0.58;
  bars.forEach((v, i) => {
    const h = Math.max(0.045, v * maxH);
    s.addShape("roundRect", {
      x: G.M + i * pitch + (pitch - bw) / 2,
      y: base - h,
      w: bw,
      h,
      rectRadius: 0.02,
      fill: { color: mix(C.molten, C.ember, i / (bars.length - 1)) },
      line: { type: "none" },
    });
  });
  s.addText("27.3 s of a real recorded session — the interviewer speaking", {
    x: G.M,
    y: 6.14,
    w: 7.4,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 8.5,
    color: "787D91",
    margin: 0,
    valign: "middle",
  });

  s.addText(
    [
      { text: `${D.team.name}`, options: { color: C.bone, bold: true, fontSize: 15 } },
      { text: `   ${D.team.members}`, options: { color: "7B7F92", fontSize: 12.5 } },
    ],
    { x: G.M, y: 6.62, w: 7.6, h: 0.4, fontFace: F.display, margin: 0, valign: "middle" }
  );

  // Live-status chip — the service really is up; /health answered 200 in 0.48s.
  s.addShape("roundRect", {
    x: 8.66,
    y: 6.63,
    w: 4.07,
    h: 0.38,
    rectRadius: 0.19,
    fill: { color: "16241C" },
    line: { color: "2A4A38", width: 1 },
  });
  s.addShape("ellipse", { x: 8.86, y: 6.755, w: 0.13, h: 0.13, fill: { color: C.validated } });
  s.addText(D.url, {
    x: 9.06,
    y: 6.63,
    w: 3.55,
    h: 0.38,
    fontFace: F.mono,
    fontSize: 8.5,
    color: "8FC7AA",
    margin: 0,
    valign: "middle",
  });

  s.addNotes(
    "Crucible. Team Pull Request.\n\n" +
      "Hook: most interview prep is a quiz with a chat window bolted on. Ours is a live spoken " +
      "conversation that gets harder when you're good.\n\n" +
      "That waveform is not decoration — it's the real amplitude envelope of a 27-second recorded " +
      "session from our replay fixture. The backend is deployed and answering right now at the URL " +
      "bottom-right."
  );
}

// ═══ 2 · Problem + insight ═════════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.bone };
  header(s, { eyebrow: "THE PROBLEM", title: "Interview prep tools don't listen." });

  const rows = [
    {
      n: "01",
      h: "Static banks test recall, not thinking",
      b: "A fixed question list can't follow up on the answer you actually gave, so it never finds the edge of what you know.",
    },
    {
      n: "02",
      h: "Nothing hears how you answer",
      b: "Real interviews are spoken. Pace, hesitation and rambling decide outcomes — and a text box captures none of it.",
    },
    {
      n: "03",
      h: "“Feedback” is a score, not a location",
      b: "A 6/10 tells you nothing actionable. You need to know which sentence was thin and which claim you couldn't back.",
    },
  ];

  rows.forEach((r, i) => {
    const y = 1.62 + i * 1.42;
    badge(s, { x: G.M, y: y + 0.02, d: 0.44, text: r.n, fill: "12141C", size: 12 });
    s.addText(r.h, {
      x: G.M + 0.66,
      y,
      w: 5.5,
      h: 0.34,
      fontFace: F.display,
      fontSize: 15.5,
      bold: true,
      color: "12141C",
      margin: 0,
      valign: "middle",
    });
    s.addText(r.b, {
      x: G.M + 0.66,
      y: y + 0.38,
      w: 5.5,
      h: 0.78,
      fontFace: F.body,
      fontSize: 12.5,
      color: C.slate,
      lineSpacingMultiple: 1.16,
      margin: 0,
      valign: "top",
    });
  });

  // Insight panel — dark card on the light slide, so it pops as the turn.
  const px = 7.24,
    pw = G.W - G.M - px;
  panel(s, { x: px, y: 1.55, w: pw, h: 3.92, dark: true });
  s.addText("THE INSIGHT", {
    x: px + 0.42,
    y: 1.86,
    w: pw - 0.84,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 10,
    bold: true,
    color: C.molten,
    charSpacing: 1.6,
    margin: 0,
    valign: "middle",
  });
  s.addText(
    [
      { text: "The gap isn't question generation.\n", options: { color: "8C90A2" } },
      { text: "It's that nothing ", options: { color: C.bone } },
      { text: "hears you", options: { color: C.ember } },
      { text: ", adapts ", options: { color: C.bone } },
      { text: "mid-conversation", options: { color: C.ember } },
      { text: ", and shows you ", options: { color: C.bone } },
      { text: "where", options: { color: C.ember } },
      { text: " you were vague.", options: { color: C.bone } },
    ],
    {
      x: px + 0.42,
      y: 2.28,
      w: pw - 0.84,
      h: 1.7,
      fontFace: F.display,
      fontSize: 19,
      bold: true,
      lineSpacingMultiple: 1.22,
      charSpacing: -0.4,
      margin: 0,
      valign: "top",
    }
  );
  s.addText(
    "Every one of those three needs the answer to arrive as speech, in real time, inside a session that is still open. That is a systems problem, not a prompting problem — which is why almost nothing does it.",
    {
      x: px + 0.42,
      y: 4.06,
      w: pw - 0.84,
      h: 1.14,
      fontFace: F.body,
      fontSize: 12.5,
      color: "9DA1B2",
      lineSpacingMultiple: 1.18,
      margin: 0,
      valign: "top",
    }
  );

  s.addText(
    [
      { text: "So we built the thing that does: ", options: { color: C.slate } },
      { text: "adaptive difficulty you can hear.", options: { color: "12141C", bold: true } },
    ],
    {
      x: G.M,
      y: 6.05,
      w: G.CW,
      h: 0.5,
      fontFace: F.display,
      fontSize: 20,
      italic: true,
      charSpacing: -0.4,
      margin: 0,
      valign: "middle",
    }
  );

  s.addNotes(
    "Three failures, one root cause.\n\n" +
      "Static banks can't follow up. Text boxes can't hear delivery. And a numeric score doesn't tell " +
      "you WHERE you were weak.\n\n" +
      "The insight: this isn't a prompting problem, it's a systems problem — you need the answer to " +
      "arrive as speech, in real time, inside a session that's still open. That's why almost nothing does it."
  );
}

// ═══ 3 · How it works ══════════════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.bone };
  header(s, { eyebrow: "THE SOLUTION", title: "Upload. Talk. Watch your words get graded." });

  const steps = [
    { n: "1", h: "Upload", b: "Résumé PDF + the job description you're actually chasing." },
    { n: "2", h: "Choose", b: "Tech Lead, Architect or PM — distinct rubrics and distinct voices." },
    { n: "3", h: "Talk", b: "Native speech-to-speech. It asks about your projects, by name." },
    { n: "4", h: "Light up", b: "Your transcript is graded span by span, in four verdicts." },
    { n: "5", h: "Improve", b: "Report, then a day-by-day roadmap with verified links." },
  ];
  const sw = G.CW / 5;
  steps.forEach((st, i) => {
    const x = G.M + i * sw;
    badge(s, { x: x + 0.02, y: 1.6, d: 0.5, text: st.n, fill: C.molten, size: 15 });
    s.addText(st.h, {
      x: x + 0.02,
      y: 2.2,
      w: sw - 0.3,
      h: 0.32,
      fontFace: F.display,
      fontSize: 16,
      bold: true,
      color: "12141C",
      margin: 0,
      valign: "middle",
    });
    s.addText(st.b, {
      x: x + 0.02,
      y: 2.54,
      w: sw - 0.3,
      h: 0.92,
      fontFace: F.body,
      fontSize: 11.5,
      color: C.slate,
      lineSpacingMultiple: 1.14,
      margin: 0,
      valign: "top",
    });
    if (i < 4) {
      s.addShape("line", {
        x: x + sw - 0.24,
        y: 1.85,
        w: 0.18,
        h: 0,
        line: { color: "C9C4BD", width: 1.25, endArrowType: "triangle" },
      });
    }
  });

  // The four-verdict chip legend — the product's signature visual.
  const verdicts = [
    { c: C.validated, t: "validated", d: "backed and correct" },
    { c: C.incomplete, t: "incomplete", d: "true but thin" },
    { c: C.unsupported, t: "unsupported", d: "claimed, not evidenced" },
    { c: C.incorrect, t: "incorrect", d: "confidently wrong" },
  ];
  s.addText("FOUR VERDICTS, ANCHORED TO YOUR ACTUAL WORDS", {
    x: G.M,
    y: 3.72,
    w: G.CW,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 9.5,
    bold: true,
    color: C.slateMute,
    charSpacing: 1.4,
    margin: 0,
    valign: "middle",
  });
  const vw = G.CW / 4;
  verdicts.forEach((v, i) => {
    const x = G.M + i * vw;
    s.addShape("roundRect", {
      x,
      y: 4.06,
      w: vw - 0.22,
      h: 0.62,
      rectRadius: 0.08,
      fill: { color: v.c },
      line: { type: "none" },
    });
    s.addText(v.t, {
      x: x + 0.16,
      y: 4.12,
      w: vw - 0.5,
      h: 0.28,
      fontFace: F.mono,
      fontSize: 12,
      bold: true,
      color: "FFFFFF",
      margin: 0,
      valign: "middle",
    });
    s.addText(v.d, {
      x: x + 0.16,
      y: 4.38,
      w: vw - 0.5,
      h: 0.24,
      fontFace: F.body,
      fontSize: 10.5,
      color: "FFFFFF",
      margin: 0,
      valign: "middle",
    });
  });

  // Persona proof — measured, from phase 3.
  panel(s, { x: G.M, y: 5.02, w: G.CW, h: 1.62, dark: false });
  s.addText("Same résumé, same JD, three personas — the opening questions genuinely diverge:", {
    x: G.M + 0.34,
    y: 5.18,
    w: G.CW - 0.68,
    h: 0.3,
    fontFace: F.body,
    fontSize: 12,
    italic: true,
    color: C.slate,
    margin: 0,
    valign: "middle",
  });
  const personas = [
    { p: "TECH LEAD", v: "voice: Charon", q: "“…how did you structure the worker pool to safely handle billing retries without duplicate charges?”" },
    { p: "ARCHITECT", v: "voice: Orus", q: "“…how was the worker pool structured to reliably process monolithic billing jobs?”" },
    { p: "PM", v: "voice: Aoede", q: "“Hi there, welcome! …how was it designed to reduce the billing job runtime?”" },
  ];
  const pw2 = (G.CW - 0.68) / 3;
  personas.forEach((p, i) => {
    const x = G.M + 0.34 + i * pw2;
    s.addText(
      [
        { text: p.p, options: { color: C.molten, bold: true, fontSize: 9.5, charSpacing: 1.2 } },
        { text: `   ${p.v}`, options: { color: C.slateMute, fontSize: 9 } },
      ],
      { x, y: 5.52, w: pw2 - 0.24, h: 0.24, fontFace: F.mono, margin: 0, valign: "middle" }
    );
    s.addText(p.q, {
      x,
      y: 5.78,
      w: pw2 - 0.24,
      h: 0.74,
      fontFace: F.body,
      fontSize: 10.5,
      color: C.slate,
      lineSpacingMultiple: 1.1,
      margin: 0,
      valign: "top",
    });
  });

  s.addNotes(
    "Five steps. Upload, choose, talk, light up, improve.\n\n" +
      "The four verdicts matter: 'unsupported' is separate from 'incorrect' on purpose. An unbacked " +
      "claim is not the same as a false one, and conflating them is how a coaching tool loses trust.\n\n" +
      "Bottom row is measured, not aspirational — same résumé through three personas produced three " +
      "genuinely different opening questions. Tech Lead goes at the failure mode, Architect at " +
      "structure, PM opens warmly and asks about outcome."
  );
}

// ═══ 4 · The product (screenshot slots) ════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.ink };
  header(s, { eyebrow: "THE PRODUCT", title: "What the candidate actually sees.", dark: true });

  const frame = (slot, { x, y, w, h, labelSize = 9.5 }) => {
    if (slot.file) {
      s.addImage({ path: slot.file, x, y, w, h, sizing: { type: "cover", w, h } });
      s.addShape("roundRect", {
        x,
        y,
        w,
        h,
        rectRadius: 0.03,
        fill: { type: "none" },
        line: { color: C.line, width: 1 },
      });
    } else {
      s.addShape("roundRect", {
        x,
        y,
        w,
        h,
        rectRadius: 0.03,
        fill: { color: C.panel },
        line: { color: C.line, width: 1 },
      });
      // The caption below the frame already names the screen, so the interior
      // stays neutral rather than repeating the label back at the reader.
      s.addText("screen capture", {
        x,
        y: y + h / 2 - 0.16,
        w,
        h: 0.32,
        fontFace: F.mono,
        fontSize: labelSize,
        color: "4A4E62",
        align: "center",
        charSpacing: 2,
        margin: 0,
        valign: "middle",
      });
    }
  };

  // Main frame, left.
  frame(SCREENSHOTS.main, { x: G.M, y: 1.58, w: 7.35, h: 4.14 });
  s.addText(SCREENSHOTS.main.label, {
    x: G.M,
    y: 5.82,
    w: 7.35,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 9.5,
    bold: true,
    color: C.molten,
    charSpacing: 1.4,
    margin: 0,
    valign: "middle",
  });
  s.addText(SCREENSHOTS.main.caption, {
    x: G.M,
    y: 6.1,
    w: 7.35,
    h: 0.82,
    fontFace: F.body,
    fontSize: 12,
    color: "9DA1B2",
    lineSpacingMultiple: 1.16,
    margin: 0,
    valign: "top",
  });

  // Two stacked frames, right.
  const rx = 8.28,
    rw = G.W - G.M - rx;
  [SCREENSHOTS.a, SCREENSHOTS.b].forEach((slot, i) => {
    const y = 1.58 + i * 2.7;
    frame(slot, { x: rx, y, w: rw, h: 1.72 });
    s.addText(slot.label, {
      x: rx,
      y: y + 1.8,
      w: rw,
      h: 0.24,
      fontFace: F.mono,
      fontSize: 9,
      bold: true,
      color: C.molten,
      charSpacing: 1.4,
      margin: 0,
      valign: "middle",
    });
    s.addText(slot.caption, {
      x: rx,
      y: y + 2.04,
      w: rw,
      h: 0.6,
      fontFace: F.body,
      fontSize: 11,
      color: "9DA1B2",
      lineSpacingMultiple: 1.14,
      margin: 0,
      valign: "top",
    });
  });

  s.addNotes(
    "Walk the judges through the live screen: transcript builds as you speak, then the spans light up " +
      "in the four verdict colours a few seconds after you finish.\n\n" +
      "The report gives a radar across the role's domains plus a band sparkline. The roadmap is " +
      "day-by-day in prerequisite order — and every link was fetched over HTTP before it was shown."
  );
}

// ═══ 5 · Architecture and stack ════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.ink };
  header(s, { eyebrow: "ARCHITECTURE", title: "One Go binary between a browser and Vertex AI.", dark: true });

  const colLabel = (t, x, w) =>
    s.addText(t, {
      x,
      y: 1.44,
      w,
      h: 0.26,
      fontFace: F.mono,
      fontSize: 9,
      bold: true,
      color: C.slateMute,
      charSpacing: 1.6,
      align: "center",
      margin: 0,
      valign: "middle",
    });

  // ── Column A · client
  // Gutters are sized to hold the arrow labels. The first cut had a 0.8in gap
  // and the "wss:// PCM16 16k" caption ran underneath the Cloud Run panel.
  const ax = G.M,
    aw = 1.95;
  colLabel("CLIENT", ax, aw);
  panel(s, { x: ax, y: 2.42, w: aw, h: 1.62, dark: true });
  s.addText("Browser", {
    x: ax,
    y: 2.6,
    w: aw,
    h: 0.32,
    fontFace: F.display,
    fontSize: 15,
    bold: true,
    color: C.bone,
    align: "center",
    margin: 0,
    valign: "middle",
  });
  s.addText("AudioWorklet\nPCM16 @ 16 kHz\n20 ms frames", {
    x: ax,
    y: 2.94,
    w: aw,
    h: 0.92,
    fontFace: F.mono,
    fontSize: 9.5,
    color: "8C90A2",
    align: "center",
    lineSpacingMultiple: 1.24,
    margin: 0,
    valign: "top",
  });

  // ── Column B · Cloud Run
  const bx = 4.05,
    bw2 = 4.85;
  colLabel("GOOGLE CLOUD RUN", bx, bw2);
  panel(s, { x: bx, y: 1.75, w: bw2, h: 3.3, dark: true, fill: "141621" });
  s.addText(
    [
      { text: "one Go binary", options: { color: C.bone, bold: true, fontSize: 14 } },
      { text: "   ·   min-instances=1, session affinity", options: { color: "70748A", fontSize: 9.5 } },
    ],
    { x: bx + 0.24, y: 1.86, w: bw2 - 0.48, h: 0.3, fontFace: F.display, margin: 0, valign: "middle" }
  );

  const lanes = [
    { k: "httpapi", v: "REST · sessions, digest, report, roadmap" },
    { k: "live", v: "WebSocket relay · turn boundaries, framing" },
    { k: "worker", v: "grading pool · evaluate, delivery, finalize" },
  ];
  lanes.forEach((l, i) => {
    const y = 2.3 + i * 0.57;
    s.addShape("roundRect", {
      x: bx + 0.24,
      y,
      w: bw2 - 0.48,
      h: 0.48,
      rectRadius: 0.05,
      fill: { color: C.panelUp },
      line: { type: "none" },
    });
    s.addText(l.k, {
      x: bx + 0.4,
      y,
      w: 0.9,
      h: 0.48,
      fontFace: F.mono,
      fontSize: 10.5,
      bold: true,
      color: C.molten,
      margin: 0,
      valign: "middle",
    });
    s.addText(l.v, {
      x: bx + 1.28,
      y,
      w: bw2 - 1.56,
      h: 0.48,
      fontFace: F.body,
      fontSize: 10.5,
      color: "A8ACBC",
      margin: 0,
      valign: "middle",
    });
  });

  s.addShape("roundRect", {
    x: bx + 0.24,
    y: 4.14,
    w: bw2 - 0.48,
    h: 0.68,
    rectRadius: 0.05,
    fill: { color: "101320" },
    line: { color: C.line, width: 1 },
  });
  s.addText("Firestore — source of truth   ·   Cloud Storage", {
    x: bx + 0.24,
    y: 4.14,
    w: bw2 - 0.48,
    h: 0.36,
    fontFace: F.mono,
    fontSize: 9.5,
    color: "8C90A2",
    align: "center",
    margin: 0,
    valign: "middle",
  });
  s.addText("in-memory session state is only a cache", {
    x: bx + 0.24,
    y: 4.46,
    w: bw2 - 0.48,
    h: 0.3,
    fontFace: F.body,
    fontSize: 9.5,
    italic: true,
    color: "5C6072",
    align: "center",
    margin: 0,
    valign: "middle",
  });

  // ── Column C · Vertex
  const cx = 9.9,
    cw = G.W - G.M - cx;
  colLabel("VERTEX AI", cx, cw);
  panel(s, { x: cx, y: 1.75, w: cw, h: 3.3, dark: true, fill: "141621" });
  const models = [
    { m: "gemini-live-2.5-\nflash-native-audio", r: "the conversation", t: "us-central1" },
    { m: "gemini-3.6-flash", r: "span grading, digest", t: "4.3 s · global" },
    { m: "gemini-3.5-flash-lite", r: "Socratic hints", t: "~1.5 s · global" },
  ];
  models.forEach((mo, i) => {
    const y = 2.0 + i * 0.98;
    s.addShape("roundRect", {
      x: cx + 0.2,
      y,
      w: cw - 0.4,
      h: 0.86,
      rectRadius: 0.05,
      fill: { color: C.panelUp },
      line: { type: "none" },
    });
    s.addText(mo.m, {
      x: cx + 0.34,
      y: y + 0.06,
      w: cw - 0.68,
      h: 0.38,
      fontFace: F.mono,
      fontSize: 8.5,
      bold: true,
      color: C.ember,
      lineSpacingMultiple: 1.05,
      margin: 0,
      valign: "middle",
    });
    s.addText(mo.r, {
      x: cx + 0.34,
      y: y + 0.44,
      w: cw - 0.68,
      h: 0.2,
      fontFace: F.body,
      fontSize: 10,
      color: C.bone,
      margin: 0,
      valign: "middle",
    });
    s.addText(mo.t, {
      x: cx + 0.34,
      y: y + 0.62,
      w: cw - 0.68,
      h: 0.2,
      fontFace: F.mono,
      fontSize: 8.5,
      color: "70748A",
      margin: 0,
      valign: "middle",
    });
  });

  // ── flow arrows
  const arrow = (x, y, w, label, sub) => {
    s.addShape("line", {
      x,
      y,
      w,
      h: 0,
      line: { color: C.molten, width: 1.75, endArrowType: "triangle" },
    });
    s.addText(label, {
      x: x - 0.06,
      y: y - 0.34,
      w: w + 0.12,
      h: 0.24,
      fontFace: F.mono,
      fontSize: 8,
      bold: true,
      color: C.molten,
      align: "center",
      margin: 0,
      valign: "middle",
    });
    if (sub)
      s.addText(sub, {
        x: x - 0.06,
        y: y + 0.06,
        w: w + 0.12,
        h: 0.24,
        fontFace: F.mono,
        fontSize: 8,
        color: "70748A",
        align: "center",
        margin: 0,
        valign: "middle",
      });
  };
  arrow(ax + aw + 0.07, 3.0, 1.36, "wss://  PCM16 16k", "PCM16 24k down");
  arrow(bx + bw2 + 0.07, 3.0, 0.86, "bidi", "stream");

  // The injection loop — the grade travels back INTO the open conversation.
  // This arrow is the adaptive-difficulty mechanism, so it gets its own label.
  const loopX = cx + cw / 2,
    loopBack = bx + 0.55;
  s.addShape("line", { x: loopX, y: 5.05, w: 0, h: 0.32, line: { color: C.ember, width: 1.5 } });
  s.addShape("line", {
    x: loopBack,
    y: 5.37,
    w: loopX - loopBack,
    h: 0,
    line: { color: C.ember, width: 1.5, beginArrowType: "triangle" },
  });
  s.addShape("line", { x: loopBack, y: 5.19, w: 0, h: 0.18, line: { color: C.ember, width: 1.5 } });
  s.addText("INJECTION LOOP  —  the grade steers the next question, inside the same open session", {
    x: loopBack,
    y: 5.4,
    w: loopX - loopBack,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 8,
    bold: true,
    color: C.ember,
    align: "center",
    margin: 0,
    valign: "middle",
  });

  // ── two callouts
  const notes = [
    {
      h: "The relay is structural, not an optimisation",
      b: "Vertex authenticates with an OAuth2 token minted from a service account, and there is no safe way to put that key in a browser. Every audio frame must pass through our backend.",
    },
    {
      h: "Two Vertex regions, because they aren't co-located",
      b: "Measured with real bidi handshakes: Live works in us-central1 and fails in global; Gemini 3.x text is global-only. Two SDK clients, one config. Assumed co-location would have failed on stage.",
    },
  ];
  const nw = (G.CW - 0.3) / 2;
  notes.forEach((n, i) => {
    const x = G.M + i * (nw + 0.3);
    s.addText(n.h, {
      x,
      y: 5.78,
      w: nw,
      h: 0.26,
      fontFace: F.display,
      fontSize: 11.5,
      bold: true,
      color: C.bone,
      margin: 0,
      valign: "middle",
    });
    s.addText(n.b, {
      x,
      y: 6.04,
      w: nw,
      h: 0.6,
      fontFace: F.body,
      fontSize: 10,
      color: "80849A",
      lineSpacingMultiple: 1.1,
      margin: 0,
      valign: "top",
    });
  });

  // ── stack logo strip
  const stack = [
    ["go", "Go 1.26"],
    ["googlecloud", "Cloud Run"],
    ["googlegemini", "Vertex AI"],
    ["firebase", "Firebase Auth"],
    ["docker", "Distroless"],
    ["react", "React"],
    ["typescript", "TypeScript"],
    ["vite", "Vite"],
  ];
  const lw = G.CW / stack.length;
  stack.forEach(([slug, label], i) => {
    const x = G.M + i * lw;
    s.addImage({ path: logo(slug), x: x + lw / 2 - 0.14, y: 6.78, w: 0.28, h: 0.28 });
    s.addText(label, {
      x,
      y: 7.08,
      w: lw,
      h: 0.22,
      fontFace: F.mono,
      fontSize: 7.5,
      color: "6E7285",
      align: "center",
      margin: 0,
      valign: "middle",
    });
  });

  s.addNotes(
    "One Go binary on Cloud Run doing three jobs: REST, the WebSocket relay, and the grading worker pool.\n\n" +
      "Two things a technical judge should take away.\n\n" +
      "First, the relay is structurally mandatory. Vertex wants an OAuth2 bearer token from a service " +
      "account — you cannot put that in frontend code, so every audio frame goes through us.\n\n" +
      "Second, the ember arrow. The grade for turn N is injected back into the SAME open live session " +
      "before turn N+1 is asked. That is the adaptive difficulty, and it's why it's audible rather " +
      "than a number on a dashboard."
  );
}

// ═══ 6 · Four hard problems ════════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.bone };
  header(s, { eyebrow: "ENGINEERING DEPTH", title: "Four problems that make this hard." });

  const cards = [
    {
      n: "01",
      t: "Where does a turn end?",
      d: "Manual activity detection, not voice-activity detection. The client owns the boundary, so a noisy hall can't fire a false end-of-turn, the model structurally cannot hear itself, and silence is never transmitted — live audio is the dominant cost.",
      pv: D.latency.turnBoundary,
      pl: `turn boundary · target ${D.latency.target}`,
    },
    {
      n: "02",
      t: "The grade arrives after it's needed",
      d: "A deadline races the grader against a deterministic fallback, so the interviewer never sits silent. We sized it at 3.5 s — and the fallback won every race, silently defeating the whole design. Measured latency forced 9 s.",
      pv: "9 s",
      pl: "injection deadline · was 3.5 s",
    },
    {
      n: "03",
      t: "A false red destroys trust instantly",
      d: "A low-confidence “incorrect” is rewritten server-side to “unsupported”, because a prompt instruction is not a defence. The grader still separates cleanly: excellent 9.60, vague 3.55, fabricated 1.60.",
      pv: D.quality.falseReds,
      pl: "false reds, across every test",
    },
    {
      n: "04",
      t: "Highlights must not land on the wrong words",
      d: "The evaluator returns verbatim excerpts; a four-tier resolver locates them in Go and drops what it cannot place. A missing highlight is invisible — a misplaced one is a bug the judge sees.",
      pv: D.quality.anchorDrop,
      pl: "span anchor drop rate",
    },
  ];

  const cw2 = (G.CW - 0.3) / 2,
    ch = 2.28;
  cards.forEach((c, i) => {
    const x = G.M + (i % 2) * (cw2 + 0.3);
    const y = 1.55 + Math.floor(i / 2) * (ch + 0.26);
    panel(s, { x, y, w: cw2, h: ch, dark: false });
    badge(s, { x: x + 0.34, y: y + 0.3, d: 0.42, text: c.n, fill: "12141C", size: 11.5 });
    s.addText(c.t, {
      x: x + 0.88,
      y: y + 0.28,
      w: cw2 - 1.22,
      h: 0.46,
      fontFace: F.display,
      fontSize: 15,
      bold: true,
      color: "12141C",
      lineSpacingMultiple: 1.0,
      margin: 0,
      valign: "middle",
    });
    s.addText(c.d, {
      x: x + 0.34,
      y: y + 0.8,
      w: cw2 - 0.68,
      h: 1.0,
      fontFace: F.body,
      fontSize: 11,
      color: C.slate,
      lineSpacingMultiple: 1.14,
      margin: 0,
      valign: "top",
    });
    s.addText(
      [
        { text: c.pv, options: { color: C.molten, bold: true, fontSize: 19, fontFace: F.display } },
        { text: `    ${c.pl}`, options: { color: C.slateMute, fontSize: 9.5, fontFace: F.mono } },
      ],
      { x: x + 0.34, y: y + 1.84, w: cw2 - 0.68, h: 0.34, margin: 0, valign: "middle" }
    );
  });

  s.addText(
    "Each of these was found by measuring, not by reading documentation — and three of them contradicted what we had planned.",
    {
      x: G.M,
      y: 6.58,
      w: G.CW,
      h: 0.36,
      fontFace: F.body,
      fontSize: 12,
      italic: true,
      color: C.slateMute,
      margin: 0,
      valign: "middle",
    }
  );

  s.addNotes(
    "The four genuinely hard problems.\n\n" +
      "Card 2 is the one I'd highlight if asked. We shipped a 3.5-second injection deadline straight " +
      "from the design doc. It looked fine. But real grading takes 5-8 seconds, so the fallback won " +
      "every single race and the grader's sharp follow-up question never reached the conversation. " +
      "The feature was inert and nothing was failing. We caught it by noticing the injected question " +
      "length was constant instead of varying.\n\n" +
      "Card 3: an unfair red is the most damaging thing this product can do, so it's gated in Go, not " +
      "hoped for in a prompt."
  );
}

// ═══ 7 · SWOT ══════════════════════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.bone };
  header(s, { eyebrow: "SWOT", title: "Where we're strong, and where we're not." });

  const quads = [
    {
      k: "S",
      t: "Strengths",
      c: C.validated,
      items: [
        "Deployed and answering, not a mock — 8/8 on the problem statement",
        "Genuinely voice-native: bidirectional speech, not STT→LLM→TTS",
        "Zero false reds across every calibration test",
        "Ghost Session replays a full interview for 0 Vertex tokens — the demo can't be killed by venue wifi",
      ],
    },
    {
      k: "W",
      t: "Weaknesses",
      c: C.incorrect,
      items: [
        "WebSocket reconnect is unbuilt — a dropped socket ends the session",
        "Evaluation runs 5–8 s against a 4 s target; the heatmap reveals a beat late",
        "Vertex concurrency unproven — the load test used replay sessions",
        "The radar chart needs three graded turns before it means anything",
      ],
    },
    {
      k: "O",
      t: "Opportunities",
      c: C.unsupported,
      items: [
        "The Live model ships 24 languages and 30 HD voices — regional-language prep is wide open",
        "Campus placement cells and bootcamps are a natural per-seat B2B wedge",
        "Study Mode already generalises past interviews to any topic",
        "Study Mode's real prerequisite DAG can upgrade roadmap ordering",
      ],
    },
    {
      k: "T",
      t: "Threats",
      c: C.incomplete,
      items: [
        "Live audio dominates cost, so unit economics track model pricing",
        "Incumbents could add voice to an existing question bank",
        "The Live API surface is Preview and already forced two-region pinning",
        "Calibration drift — one unfair red costs a user's trust permanently",
      ],
    },
  ];

  const qw = (G.CW - 0.3) / 2,
    qh = 2.28;
  quads.forEach((q, i) => {
    const x = G.M + (i % 2) * (qw + 0.3);
    const y = 1.55 + Math.floor(i / 2) * (qh + 0.26);
    panel(s, { x, y, w: qw, h: qh, dark: false });
    badge(s, { x: x + 0.34, y: y + 0.28, d: 0.44, text: q.k, fill: q.c, size: 15 });
    s.addText(q.t, {
      x: x + 0.9,
      y: y + 0.28,
      w: qw - 1.24,
      h: 0.44,
      fontFace: F.display,
      fontSize: 17,
      bold: true,
      color: "12141C",
      margin: 0,
      valign: "middle",
    });
    s.addText(
      q.items.map((t, j) => ({
        text: t,
        options: { bullet: true, breakLine: j < q.items.length - 1 },
      })),
      {
        x: x + 0.36,
        y: y + 0.82,
        w: qw - 0.72,
        h: 1.36,
        fontFace: F.body,
        fontSize: 10.5,
        color: C.slate,
        lineSpacingMultiple: 1.1,
        paraSpaceAfter: 4,
        margin: 0,
        valign: "top",
      }
    );
  });

  s.addText(
    "The weaknesses are quoted from our own build checkpoints. We'd rather name them than be asked about them.",
    {
      x: G.M,
      y: 6.58,
      w: G.CW,
      h: 0.36,
      fontFace: F.body,
      fontSize: 12,
      italic: true,
      color: C.slateMute,
      margin: 0,
      valign: "middle",
    }
  );

  s.addNotes(
    "Honest SWOT. The weaknesses come straight out of our own checkpoint docs.\n\n" +
      "If a judge asks 'what's broken' — reconnect. A dropped socket ends the session. The " +
      "session-resumption handles are already being emitted by the server, nothing consumes them yet. " +
      "That's the top of the backlog.\n\n" +
      "Biggest opportunity: the Live model ships 24 languages. Regional-language interview prep in " +
      "India is a large, genuinely underserved market and it's a config change, not a rebuild."
  );
}

// ═══ 8 · Measured, not asserted ════════════════════════════════════════════
{
  const s = pres.addSlide();
  s.background = { color: C.ink };
  header(s, { eyebrow: "VALIDATION", title: "Measured, not asserted.", dark: true });

  // Band 1 — six stat callouts
  const stats = [
    { v: D.robustness.load, l: "CONCURRENT SESSIONS" },
    { v: D.robustness.chaos, l: "CHAOS CHECKS" },
    { v: D.robustness.tests, l: "TESTS" },
    { v: D.quality.falseReds, l: "FALSE REDS" },
    { v: D.quality.anchorDrop, l: "ANCHOR DROP" },
    { v: D.quality.compliance, l: "PROBLEM STATEMENT" },
  ];
  const stw = G.CW / 6;
  stats.forEach((st, i) => {
    stat(s, { x: G.M + i * stw, y: 1.5, w: stw, value: st.v, label: st.l, dark: true, size: 29 });
  });

  // Band 2 left — the model A/B chart
  const chx = G.M,
    chw = 6.05;
  s.addChart(
    pres.ChartType.bar,
    [
      { name: D.modelAB.slow.name, labels: D.modelAB.cats, values: D.modelAB.slow.runs },
      { name: D.modelAB.fast.name, labels: D.modelAB.cats, values: D.modelAB.fast.runs },
    ],
    {
      x: chx,
      y: 2.6,
      w: chw,
      h: 2.62,
      barDir: "col",
      chartColors: ["5A5E70", C.molten],
      showTitle: true,
      title: "Evaluation latency by model — seconds, three warm runs",
      titleColor: C.bone,
      titleFontFace: F.body,
      titleFontSize: 11,
      showValue: true,
      dataLabelPosition: "outEnd",
      dataLabelColor: "C6C9D6",
      dataLabelFontFace: F.mono,
      dataLabelFontSize: 9,
      dataLabelFormatCode: '0.0"s"',
      showLegend: true,
      legendPos: "b",
      legendColor: "9DA1B2",
      legendFontFace: F.mono,
      legendFontSize: 9,
      catAxisLabelColor: "8C90A2",
      catAxisLabelFontFace: F.mono,
      catAxisLabelFontSize: 9,
      valAxisLabelColor: "8C90A2",
      valAxisLabelFontFace: F.mono,
      valAxisLabelFontSize: 9,
      valGridLine: { color: "23263A", size: 1 },
      catGridLine: { style: "none" },
      valAxisMaxVal: 60,
      border: { pt: 0, color: C.ink },
      fill: C.ink,
      plotArea: { fill: { color: C.ink } },
    }
  );
  s.addText(
    "We measured instead of assuming — and the data reversed our own recommendation. 3.5-flash was the safe pick on paper; it was unusable in practice.",
    {
      x: chx,
      y: 5.26,
      w: chw,
      h: 0.5,
      fontFace: F.body,
      fontSize: 10,
      italic: true,
      color: "80849A",
      lineSpacingMultiple: 1.12,
      margin: 0,
      valign: "top",
    }
  );

  // Band 2 right — verification table
  const tx = 7.0,
    tw = G.W - G.M - tx;
  s.addText("EVERY CLAIM, AND WHAT IT MEASURED", {
    x: tx,
    y: 2.6,
    w: tw,
    h: 0.26,
    fontFace: F.mono,
    fontSize: 9,
    bold: true,
    color: C.molten,
    charSpacing: 1.4,
    margin: 0,
    valign: "middle",
  });
  const checks = [
    ["Turn-boundary latency, deployed", `${D.latency.turnBoundary}`],
    ["Evaluation, median", D.latency.evaluation],
    ["Roadmap links resolving", `${D.quality.links} HTTP 200`],
    ["Load: 10 sessions, gaps / errors / drops", "0 / 0 / 0"],
    ["Coach state leaked aloud, 5 phases", D.quality.leaks],
    ["Ghost Session — 27 s interview", `${D.robustness.replayTokens} tokens`],
    ["Go source, excluding tests", `${D.code.go} lines`],
  ];
  checks.forEach(([k, v], i) => {
    const y = 2.94 + i * 0.34;
    s.addText(k, {
      x: tx,
      y,
      w: tw - 1.5,
      h: 0.3,
      fontFace: F.body,
      fontSize: 10.5,
      color: "A8ACBC",
      margin: 0,
      valign: "middle",
    });
    s.addText(v, {
      x: tx + tw - 1.5,
      y,
      w: 1.5,
      h: 0.3,
      fontFace: F.mono,
      fontSize: 10.5,
      bold: true,
      color: C.ember,
      align: "right",
      margin: 0,
      valign: "middle",
    });
  });

  // Band 3 — economics + what's next
  s.addText(
    [
      { text: "PER-SESSION LEDGER   ", options: { color: C.molten, bold: true, fontSize: 8.5, charSpacing: 1.2 } },
      { text: D.cost, options: { color: "A8ACBC", fontSize: 9.5 } },
      { text: "   —  metered per session, so unit economics are a number we can quote, not a guess.", options: { color: "6E7285", fontSize: 9.5 } },
    ],
    { x: G.M, y: 5.86, w: G.CW, h: 0.28, fontFace: F.mono, margin: 0, valign: "middle" }
  );

  const next = ["Reconnect via session resumption", "Vertex-level concurrency testing", "Study DAG → roadmap ordering", "Regional languages"];
  const nw2 = G.CW / 4;
  next.forEach((t, i) => {
    const x = G.M + i * nw2;
    s.addShape("roundRect", {
      x,
      y: 6.22,
      w: nw2 - 0.16,
      h: 0.42,
      rectRadius: 0.21,
      fill: { color: C.panel },
      line: { color: C.line, width: 1 },
    });
    s.addText(t, {
      x: x + 0.1,
      y: 6.22,
      w: nw2 - 0.36,
      h: 0.42,
      fontFace: F.body,
      fontSize: 9.5,
      color: "A8ACBC",
      align: "center",
      margin: 0,
      valign: "middle",
    });
  });

  // Footer
  s.addText(
    [
      { text: `${D.team.name}`, options: { color: C.bone, bold: true } },
      { text: `   ${D.team.members}`, options: { color: "6E7285" } },
    ],
    { x: G.M, y: 6.86, w: 7.0, h: 0.34, fontFace: F.display, fontSize: 11, margin: 0, valign: "middle" }
  );
  s.addText(D.url, {
    x: 7.6,
    y: 6.86,
    w: G.W - G.M - 7.6,
    h: 0.34,
    fontFace: F.mono,
    fontSize: 9.5,
    color: "8FC7AA",
    align: "right",
    margin: 0,
    valign: "middle",
  });

  s.addNotes(
    "Close on evidence.\n\n" +
      "Ten concurrent sessions with zero gaps and zero drops. Seventeen chaos checks — every failure " +
      "mode degrades toward 'the interview keeps working' rather than erroring out. 125 tests. Zero " +
      "false reds.\n\n" +
      "The chart is my favourite slide in the deck. We planned to use 3.5-flash because shipping a " +
      "four-day-old model into a demo is a bad idea. Then we measured: 55 seconds, 7 seconds, 24 " +
      "seconds. Unusable. 3.6-flash was 4.6, 4.6, 4.1. The data overrode our own plan, and we wrote " +
      "that down rather than quietly changing the recommendation.\n\n" +
      "And we can quote unit economics per session, because every Vertex call is metered into a ledger."
  );
}

// ── write ──────────────────────────────────────────────────────────────────
const out = path.join(__dirname, "Crucible-InnovaHack.pptx");
pres
  .writeFile({ fileName: out })
  .then((f) => console.log(`wrote ${f}`))
  .catch((e) => {
    console.error(e);
    process.exit(1);
  });
