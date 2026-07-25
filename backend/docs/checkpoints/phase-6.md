# Phase 6 checkpoint — Report and delivery metrics

**Status: complete.**

> **Exit criterion:** a deliberately disfluent answer produces a non-zero filler
> count (PRD risk R7). ✅ Met — 12–13 fillers on a 15-second disfluent answer.

Full pipeline verified end to end: spoken answer → evaluation → delivery
analysis → finalization → report.

```
turn evaluated     score 2.25, 2 spans, 0 dropped
delivery analysed  156 wpm (optimal), 13 fillers, hesitation 0.85
report generated   overall 2.3, 5 radar domains, 4 gaps
```

## ⚠️ The PRD is wrong about disfluencies, and it changes the justification

PRD §13.1 is emphatic: *"Google's speech recognition normalises disfluencies out
of the transcript… If you count fillers by regex over the Live API transcript,
you will ship a counter that always reads zero."*

**Measured against the Live API's input transcription, that is not true.** The
transcript for our disfluent fixture came back as:

> "So um, back pressure, uh, we'd sort of like handled it at the um Q level,
> I think and uh you know, we had like some monitoring um set up. It uh
> basically worked. I mean mostly, I'm fine."

A regex over that finds **6 hard fillers** (um/uh) and **6 soft hedges** — twelve
in total, against the audio analysis's thirteen. A transcript-based counter
would not have read zero. It would have been roughly right.

**The audio call is still the correct design, but for different reasons than the
PRD gives:**

1. **Hesitation (0.80–0.85) is prosodic.** Pauses mid-sentence, restarts,
   trailing off, rising intonation on statements — no transcript carries any of
   it, at any quality level. This is the signal that genuinely cannot be
   recovered from text.
2. **Disambiguating hedges needs the ear.** "Like a bloom filter" is a
   comparison; "like, handled it" is filler. A regex cannot tell them apart and
   will over-count.
3. **Transcript behaviour is not contractual.** It varies by model, version, and
   configuration. Building the feature on a behaviour that happens to hold today
   is fragile in exactly the way the PRD was trying to warn about.

The integration test now compares against the **real** transcript rather than a
tidied one I wrote assuming the PRD was right — that earlier version flattered
the result — and asserts on hesitation, which is the property that actually
justifies the call.

## What exists now

| Component | Purpose |
|---|---|
| `internal/delivery` | Audio-based pace and disfluency analysis, degrading to deterministic metrics. 3 live tests. |
| `internal/report` | Deterministic aggregation: radar, sparkline, strengths/gaps, per-turn accordion. 9 unit tests. |
| `internal/grading/finalize.go` | `JobDeliveryMetrics` and `JobFinalize` handlers. |
| `internal/httpapi/report.go` | `GET /report` with the 202-polling contract, `GET /turns`. |

`go vet` clean, **106 unit tests across 10 packages**, plus 7 live integration tests.

## The bug that mattered: turns were closing empty

`closeTurn()` fired the moment `activity_end` arrived from the client. But the
user's transcript arrives from Vertex **asynchronously and later**, produced by
a different goroutine once the audio has been processed.

So every voice turn was snapshotted with an empty transcript and then skipped as
*"answer too short, words: 0"* — despite fifteen seconds of speech. The report
showed `turnsGraded: 0`. Voice interviews were being graded not at all, while
text answers (whose transcript is known synchronously) worked fine and masked it.

Fixed with a `boundaryPending` flag: `activity_end` marks the boundary, and the
turn closes when the transcription lands — with `TurnComplete` as a backstop for
silence or a recognition failure. Closing on transcription rather than on
`TurnComplete` also means grading starts while the interviewer is still
acknowledging, rather than after it has finished speaking.

## A bug my own test caught

`domainMatches` was documented as word-level but implemented with
`strings.Contains`, so "serving" matched "observing" and a turn about metrics
would have been attributed to a *model serving* radar axis. The test asserting
the documented behaviour failed on the first run. Now tokenises properly, with a
plural/gerund allowance so "feature pipelines" still matches "feature pipeline".

## Design notes

- **The report makes no model call.** Every judgement was made per-turn by the
  evaluator; re-asking a model to summarise its own summaries would add latency,
  cost, and a fresh chance to contradict itself.
- **Deterministic and inferred are strictly separated.** WPM, speaking time, and
  word count are arithmetic in Go. Only fillers, hesitation, and pace character
  come from the model.
- **Gaps are capped at five and ranked by frequency.** A list of nineteen
  weaknesses is not actionable, it is discouraging.
- **Unattributable turns spread across every radar axis** rather than being
  discarded. A session whose concepts never name a domain would otherwise render
  a blank chart, which reads as broken rather than empty.
- **Delivery feedback is behaviour, never character.** A test asserts the output
  never contains "unconfident", "nervous", "you seem" and similar. This is where
  a coaching tool most easily becomes cruel, and a character judgement is
  something the person cannot act on.
- **Finalization waits up to 45 s for outstanding grades**, then proceeds. A
  stuck grader must not block the report forever; an ungraded turn renders
  honestly rather than as a missing row.

## Honest caveats

- **The radar chart is flat on a single-turn session.** With one unattributable
  turn every axis shows the same score. It differentiates only across several
  turns touching different domains. Real for the demo: a three-turn session is
  the minimum for a chart worth showing.
- **`fillerPerMinute` reads high on short answers** (51.9/min on a 15-second
  answer). Arithmetically correct, but a per-minute rate extrapolated from
  fifteen seconds overstates. The report should probably show the raw count
  prominently and the rate as secondary.
- **Delivery analysis costs one extra model call per turn with audio.** PRD
  §21.1 rates this "moderate" and it is queued only when audio exists, so typed
  answers cost nothing.

## Notes for Phase 7

- `Report.Gaps` is the roadmap's input — already ranked by frequency and capped.
- `Session.Coverage.Missing` carries the fuller list if the roadmap wants more
  than five.
- `JobBuildRoadmap` is already queued by `handleFinalize` once the report is
  written, and is currently unregistered — Phase 7 registers the handler.
- The roadmap needs Search grounding: `tools: [{googleSearch: {}}]`. One
  grounded call for the whole roadmap, not one per day.
