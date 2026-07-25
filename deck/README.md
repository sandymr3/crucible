# Crucible — InnovaHack deck

Eight slides, generated from code. `Crucible-InnovaHack.pptx` is the deliverable.

```bash
npm install              # pptxgenjs + sharp
node fetch-assets.js     # brand logos -> assets/
node build-deck.js       # -> Crucible-InnovaHack.pptx
```

## Dropping in the frontend screenshots

Slide 4 reserves three frames. Edit the `SCREENSHOTS` object at the top of
[`build-deck.js`](build-deck.js), set `file` to an image path, and re-run:

```js
const SCREENSHOTS = {
  main: { file: "shots/interview.png", label: "LIVE INTERVIEW", caption: "…" },
  a:    { file: "shots/report.png",    label: "REPORT",         caption: "…" },
  b:    { file: "shots/roadmap.png",   label: "ROADMAP",        caption: "…" },
};
```

The image lands in the identical rect the placeholder occupied, so nothing else
moves. Shoot them 16:10-ish; the frames crop with `sizing: cover`.

## Where the numbers come from

Every figure on every slide is read from [`facts.js`](facts.js), and every entry
there is a **measured** value from `backend/docs/checkpoints/phase-0..9.md`.
Nothing is estimated. If a number needs to change, change it there — do not type
one into a slide.

The title-slide waveform is the real amplitude envelope of the 27.29-second
recorded session in `backend/internal/replay/fixtures/demo-ml-engineer.json`,
reduced to 72 RMS buckets. The near-flat bars are genuine pauses in the speech.

## QA

```bash
node build-deck.js
powershell -File export-png.ps1     # -> render/Slide1..8.PNG
PYTHONUTF8=1 python <pptx-skill>/scripts/office/validate.py Crucible-InnovaHack.pptx
```

Two environment notes, both discovered the hard way:

- **LibreOffice and `pdftoppm` are not installed on this machine**, so the usual
  render path doesn't work. `export-png.ps1` drives **PowerPoint COM** instead,
  which is strictly better here: it's the renderer the judges will use, so font
  metrics are exact and apparent text fit can be trusted.
- **`validate.py` needs `PYTHONUTF8=1` on Windows.** Without it the script opens
  slide XML as cp1252 and dies on the deck's curly quotes — that failure is the
  validator's encoding bug, not a defect in the deck.

`export-png.ps1` waits for `POWERPNT.EXE` to exit before returning. Without that
wait the next `node build-deck.js` fails with `EBUSY` on a file that nothing
appears to own.

## Design system

[`theme.js`](theme.js) — "Forge": near-black ink dominant, one molten accent,
ember for numerals. The four span-verdict colours are **data only** and never
decorative, because they're the product's own visual language.

Fonts are Arial / Calibri / Consolas — all ship with Office on Windows and Mac,
so nothing substitutes on a presenting machine.

## Assets

Brand marks come from [Simple Icons](https://simpleicons.org) (CC0), recoloured
to a single tint at fetch time so the logo strip reads as one designed row. The
marks themselves remain their owners'. Every diagram, the waveform, the SWOT
quadrants and the chart are native PowerPoint shapes — no rasterised diagrams,
and no stock photography.
