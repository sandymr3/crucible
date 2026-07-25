# Crucible — documentation

**An adaptive, voice-native AI interview and study coach, built entirely on Vertex AI.**

InnovaHack Chapter-1 · Round 1 · Gen AI Problem Statement 2
Team **PULL REQUEST** — Santhosh P · Hrithik Sankar R

| | |
|---|---|
| **Live backend** | https://crucible-backend-103350253775.us-central1.run.app |
| **Health check** | [`/health`](https://crucible-backend-103350253775.us-central1.run.app/health) → `{"status":"ok"}` |
| **Repository** | https://github.com/sandymr3/crucible |
| **Pitch deck** | [`deck/Crucible-InnovaHack.pptx`](deck/Crucible-InnovaHack.pptx) |

---

## In sixty seconds

Most interview prep is a quiz generator with a chat window bolted on. Crucible is
a **live spoken conversation**. You upload your résumé and the job description
you're actually chasing, pick who's grilling you, and then you talk. The AI talks
back in its own voice, in real time, asking questions rooted in the projects on
your résumé.

When you finish an answer, your own words light up on screen — green where you
nailed it, amber where you were vague, blue where you claimed something you
couldn't support. That grade is then injected back into the *same open session*,
so the next question is genuinely harder or genuinely easier.

That last part is the point: **adaptive difficulty you can hear.**

---

## Where each evaluation criterion is satisfied

| # | Criterion | Marks | What satisfies it | Read |
|---|---|---|---|---|
| 1 | Problem Understanding & Solution | 20 | All eight required capabilities implemented and verified, 8/8 | [01-problem-and-solution.md](01-problem-and-solution.md) |
| 2 | Innovation & Creativity | 15 | Eight architecture decisions, four of them genuinely novel | [02-innovation.md](02-innovation.md) |
| 3 | Technical Implementation | 25 | 24 packages · 11,912 lines of Go · 24 routes · bidirectional audio relay | [03-architecture.md](03-architecture.md) · [04-api-reference.md](04-api-reference.md) · [05-security-and-guardrails.md](05-security-and-guardrails.md) |
| 4 | Completeness & Functionality | 15 | Every backend capability working end to end on the deployed service | [06-testing-and-validation.md](06-testing-and-validation.md) |
| 5 | Presentation & Demo | 10 | Deck, plus a timed demo script with a network-proof fallback | [09-demo-script.md](09-demo-script.md) · [deck/](deck/) |
| 6 | Scalability & Real-World Impact | 10 | 10/10 concurrent verified · stateless instances · per-session unit economics | [07-scalability-and-impact.md](07-scalability-and-impact.md) |
| 7 | Documentation & Submission Quality | 5 | This folder, a runnable setup guide, and a live deployment | [08-setup-and-deployment.md](08-setup-and-deployment.md) |

---

## Reading order

**If you have five minutes** — this page, then
[02-innovation.md](02-innovation.md) for what is new here, then the architecture
diagram at the top of [03-architecture.md](03-architecture.md).

**If you have thirty** — add [05-security-and-guardrails.md](05-security-and-guardrails.md)
and [06-testing-and-validation.md](06-testing-and-validation.md). Those two are
where the engineering actually shows.

**If you want to run it** — [08-setup-and-deployment.md](08-setup-and-deployment.md).

---

## Contents

| File | Covers |
|---|---|
| [01-problem-and-solution.md](01-problem-and-solution.md) | The problem, who has it, why existing tools fail, and the capability matrix |
| [02-innovation.md](02-innovation.md) | The eight architecture decisions and what each one buys |
| [03-architecture.md](03-architecture.md) | System diagram, turn lifecycle, package map, data model |
| [04-api-reference.md](04-api-reference.md) | All 24 routes and the full WebSocket frame protocol |
| [05-security-and-guardrails.md](05-security-and-guardrails.md) | Auth, isolation, Firestore rules, IAM, the nine credit guardrails |
| [06-testing-and-validation.md](06-testing-and-validation.md) | 125 tests, chaos 17/17, load 10/10, and the measured acceptance criteria |
| [07-scalability-and-impact.md](07-scalability-and-impact.md) | How it scales, what it costs per session, and who it is for |
| [08-setup-and-deployment.md](08-setup-and-deployment.md) | Run it locally, deploy it yourself |
| [09-demo-script.md](09-demo-script.md) | A timed walkthrough and the offline fallback |

Per-phase build records — including what broke and what the fix was — are in
[`../backend/docs/checkpoints/`](../backend/docs/checkpoints/).

---

## A note on the numbers

Every figure in this documentation is a **measured** value, recorded at the time
it was observed in the phase checkpoints. Where a planning assumption turned out
to be wrong — model availability, region co-location, whether speech recognition
strips disfluencies, how long grading actually takes — the checkpoints record the
measurement rather than quietly correcting the claim.

Two things are stated plainly rather than hidden: evaluation latency runs 5–8 s
against a 4 s design target, and the WebSocket reconnect path is not built. Both
appear in [06-testing-and-validation.md](06-testing-and-validation.md).

## Current status

The **backend is complete and deployed** — all nine build phases, all eight
required capabilities, verified on the live service.

The **frontend is in progress** on the `feat/frontend` branch: design tokens, six
UI primitives, and the verdict-span heatmap components are built; auth, the API
client, the audio pipeline and the screens are not yet wired. The backend is
demonstrable today through `cmd/wsprobe`, a CLI client that speaks the real
WebSocket protocol — see [09-demo-script.md](09-demo-script.md).
