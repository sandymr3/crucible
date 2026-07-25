# 01 · Problem and solution

> Evaluation criterion 1 — *Problem Understanding & Solution* (20 marks)
> Clear understanding of the problem · solution aligns with the selected domain ·
> addresses real user needs · practical and feasible approach

---

## The problem statement

**Gen AI Problem Statement 2 — Personalized AI Study / Interview Coach.** Build a
system that takes a topic or job role, generates relevant practice questions,
evaluates spoken or written answers, gives structured feedback with strengths,
gaps and suggested resources, and adapts difficulty over time.

## The problem underneath the problem statement

Interview preparation is a large, genuinely painful market, and the tools that
serve it fail in three specific ways. These are not opinions about product
polish — they are structural.

### 1. A static question bank tests recall, not thinking

A fixed list cannot follow up on the answer you actually gave. Real interviewers
do exactly one thing that matters: they hear a slightly weak answer and *push on
that spot*. A question bank cannot, so it never finds the edge of what you know —
which is the only place learning happens.

### 2. Nothing listens to *how* you answer

Real technical interviews are spoken. Outcomes turn on pace, hesitation,
rambling, and whether you can make a stranger understand you under pressure. A
text box captures none of that. Candidates who read well on paper and fall apart
out loud get no signal at all until it costs them an offer.

### 3. "Feedback" is a score, not a location

A 6/10 is not actionable. To improve, you need to know **which sentence** was
thin and **which claim** you could not back — not an aggregate. Scores tell you
that you failed; they do not tell you where.

## The insight

The gap is not question generation — large models have been good at that for
years. The gap is that nothing **hears you, adapts mid-conversation, and shows
you where you were vague.**

All three of those require the answer to arrive as *speech*, in *real time*,
inside a session that is *still open*. That is a systems problem, not a prompting
problem — which is precisely why almost nothing does it, and why most of this
project is backend engineering rather than prompt design.

## Who this is for

| User | The need it meets |
|---|---|
| Final-year students in campus placements | Structured practice against the specific JD they are chasing, without needing a senior engineer's free hour |
| Career switchers | An interviewer who probes their *actual* résumé rather than generic questions for a role they have not held |
| Anyone who freezes out loud | The only tool that gives feedback on delivery — pace, filler, hesitation — because it is the only one that hears them |
| Self-learners (Study Mode) | Any topic decomposed into a dependency-ordered syllabus, drilled to mastery |

---

## The solution

Upload a résumé PDF and the job description. Pick an interviewer. Then **talk**.

1. **Ingestion** — the résumé and JD become a *Session Digest*: extracted claims, specific probe angles, and an interview plan with target difficulty per area. Gemini reads the PDF natively, so two-column layouts, tables and design-heavy résumés work.
2. **The conversation** — a bidirectional native-audio session. The interviewer speaks in its own voice and asks about *your* projects by name.
3. **Grading** — when you finish, the transcript is graded span by span into four verdicts, anchored to your literal words.
4. **Adaptation** — the grade is injected back into the same open session, so the next question changes.
5. **The artifacts** — a report (radar across the role's domains, delivery metrics, per-turn breakdown) and a day-by-day roadmap whose every link has been fetched and verified.

### Why the approach is practical and feasible

- **It is deployed and answering right now.** Not a prototype on a laptop — Cloud Run, `min-instances=1`, a live health endpoint.
- **One binary.** REST, the WebSocket relay and the worker pool run in a single Go process. At this scale that is the entire queue infrastructure, and it removes a whole class of deployment failure.
- **Nine server-side credit guardrails**, so the running cost cannot run away — a hard session cap, a 90-second idle timeout that closes the billing connection, daily and concurrency caps.
- **It degrades toward "the interview keeps working."** Grading fails → the turn is marked ungraded and the conversation continues. Grounding fails → the roadmap ships without links and says so. Verified across 17 chaos checks.

---

## Capability matrix — 8 of 8

Every capability the problem statement asks for, and where it is implemented.

| Required capability | How Crucible satisfies it | Implementation |
|---|---|---|
| Takes a topic **or** job role as input | Interview Mode ingests a résumé PDF + job description; Study Mode decomposes a bare topic into a dependency-ordered syllabus | [`internal/ingest`](../backend/internal/ingest) · [`internal/study/syllabus.go`](../backend/internal/study/syllabus.go) |
| Generates relevant practice questions | Generated live by an interviewer persona conditioned on the résumé digest, the JD's requirements, the current difficulty band, and the concepts already proven | [`internal/persona`](../backend/internal/persona) · [`internal/study/question.go`](../backend/internal/study/question.go) |
| Evaluates **spoken** answers | Native-audio bidirectional streaming; speech transcribed by the Live model and graded span by span | [`internal/live`](../backend/internal/live) · [`internal/evaluator`](../backend/internal/evaluator) |
| Evaluates **written** answers | Typed answers and Study Mode share the identical evaluation path | [`internal/turn`](../backend/internal/turn) |
| Structured feedback — strengths | `concepts_demonstrated` aggregated into a radar chart across the role's domains | [`internal/report`](../backend/internal/report) |
| Structured feedback — gaps | `concepts_missing` plus a span-level heatmap with four verdicts, anchored server-side | [`internal/anchor`](../backend/internal/anchor) |
| Suggested resources | Roadmap generated with Google Search grounding — and **every URL is fetched and verified** before it is shown | [`internal/roadmap`](../backend/internal/roadmap) |
| Adapts difficulty | Five-band ladder with promotion and demotion rules, injected back into the live session so the *next* question genuinely changes | [`internal/difficulty`](../backend/internal/difficulty) · [`internal/grading/injection.go`](../backend/internal/grading/injection.go) |

Verified live: `BAND 3 → 4` after a strong answer, with the band change pushed
down the WebSocket so the user can see it happen.

---

## What we deliberately did *not* build

Stated so the scope reads as a decision rather than an omission:

- **No PDF text-extraction library.** Gemini reads the PDF multimodally. A text extractor would be slower to build, worse at two-column layouts, and would return empty on a scanned résumé instead of reading it.
- **No Pub/Sub, Cloud Tasks, or second runtime.** Each live session is two goroutines and a channel; the evaluation pool is a buffered channel and N workers.
- **No separate model call to summarise the report.** Every judgement was already made per-turn by the evaluator. Re-asking a model to summarise its own summaries would add latency, cost, and a fresh chance to contradict itself.

Next: [02-innovation.md](02-innovation.md) — what is actually novel here.
