# CRUCIBLE — Product Requirements Document

**An adaptive, voice-native AI interview and study coach built entirely on Vertex AI.**

| | |
|---|---|
| **Document version** | 1.0 |
| **Date** | 25 July 2026 |
| **Hackathon track** | Gen AI Problem Statement 2 — Personalized AI Study / Interview Coach |
| **Working product name** | Crucible *(placeholder — see §1.4)* |
| **Status** | Approved for build |

---

# PART I — FOUNDATION

## 1. Executive Summary

### 1.1 The pitch

> Most interview prep tools are quiz generators with a chat window bolted on. Crucible is a **live conversation**. You upload your resume and the job description you're actually chasing, pick who's grilling you — a Tech Lead, a System Architect, or a Product Manager — and then you *talk*. The AI talks back in its own voice, in real time, asking questions rooted in the projects on your resume. When you finish an answer, your own words light up on screen: green where you nailed the concept, amber where you were vague, red where you claimed something you couldn't support. At the end you get a scored breakdown, a read on how you actually *sounded* — pace, filler words, hesitation — and a day-by-day roadmap of exactly what to close before the real thing.

### 1.2 Why this wins the problem statement

The brief asks for four things. Crucible does all four, but the differentiator is that it does them **in a real conversation rather than a form submission**. Adaptive difficulty in a text quiz is a number changing in a database. Adaptive difficulty in a voice interview is an interviewer who audibly changes register when you start struggling — and that is a thing a judge feels in fifteen seconds.

### 1.3 Design principles

1. **Voice-first, not voice-optional.** The primary loop is spoken. Text is the accessibility fallback.
2. **Every score is traceable.** No opaque grades. Every number maps to a span of what the user actually said.
3. **Never punish a correct answer.** A false red flag destroys trust faster than a missed one. When the evaluator is unsure, it says *unsupported*, not *wrong*.
4. **The AI is a coach, not a gatekeeper.** Hints are Socratic. The report ends in a plan, not a verdict.
5. **Vertex-native, no exceptions.** Every token of inference is billed to the Vertex AI project. No Gemini API keys anywhere in the codebase.

### 1.4 Naming

"Crucible" is a working title used consistently through this document. Alternatives to consider before submission: **The Panel**, **Proving Ground**, **Aloud**, **Forge**. Pick one and do a find-replace — a memorable name is worth a surprising number of judging points.

---

## 2. Problem Statement Compliance Matrix

This table exists so a judge can verify coverage in under a minute. It should be reproduced verbatim on a slide.

| Required capability | How Crucible satisfies it | Where it's visible in the demo |
|---|---|---|
| **Takes a topic or job role as input** | Dual entry: **Interview Mode** ingests resume PDF + job description text; **Study Mode** ingests a bare topic string and decomposes it into a dependency-ordered syllabus | Setup screen, 0:00–0:25 of the demo |
| **Generates relevant practice questions** | Questions are generated live by the interviewer persona, conditioned on the resume digest, the JD's extracted requirements, the current difficulty band, and the set of concepts already proven | First spoken question, 0:30 |
| **Evaluates spoken answers** | Native-audio bidirectional streaming; user speech is transcribed by the Live model, and the answer audio is separately analysed for delivery quality | Right panel fills with the user's words as they speak, 0:45 |
| **Evaluates written answers** | Text input path shares the identical evaluation pipeline; Study Mode defaults to text | Study Mode toggle |
| **Structured feedback: strengths** | `concepts_demonstrated[]` per turn, aggregated into a radar chart across technical domains | Report screen, 2:10 |
| **Structured feedback: gaps** | `concepts_missing[]` per turn plus the span-level heatmap showing exactly where the answer thinned out | Heatmap reveal, 1:20 |
| **Suggested resources** | Roadmap generator uses Grounding with Google Search against a domain allowlist, so links are real and authoritative | Roadmap screen, 2:30 |
| **Adapts difficulty based on performance** | Explicit 5-band ladder with promotion/demotion rules; the band is injected back into the live session so the *next* question genuinely changes | Visible band indicator in the live room, changing on camera |

---

## 3. Personas and User Journeys

### 3.1 Primary persona — "Arjun, 22, final-year CSE"

Has a resume with three projects he can half-explain. Has an on-site with a product company in eleven days. Has done two hundred LeetCode problems and zero mock interviews, because mock interviews require a human being who is willing to be mean to him. Freezes when asked "why did you choose that?" His actual gap is not knowledge; it's the ability to *narrate* knowledge under mild adversarial pressure.

**What he needs from us:** repetition without social cost, and a specific list of what to fix.

### 3.2 Secondary persona — "Meera, 27, switching from data analyst to ML engineer"

Knows the theory from coursework. Her risk is that her resume promises more depth than she can defend. She needs the system to find the overclaims *before* an interviewer does.

**What she needs from us:** resume-grounded interrogation, and honest flagging of unsupported claims.

### 3.3 Tertiary persona — "Sana, 20, preparing for a university exam"

Not interviewing at all. Has a syllabus and four days. Needs active recall, not conversation practice.

**What she needs from us:** Study Mode — fast, text-first, topic-driven drilling with the same rigorous feedback.

### 3.4 Journey A — Interview Mode (the demo path)

```
Sign in
  → Upload resume PDF, paste JD
  → System extracts: skills, frameworks, project claims, JD requirements, gap list
  → Select interviewer persona (Tech Lead / System Architect / Product Manager)
  → Review generated "interview plan" (5 question areas) — user may drop any area
  → Enter Live Room
      ↓
  ┌─────────── LOOP (per turn) ───────────┐
  │  AI asks question aloud                │
  │  User speaks answer                    │
  │  Transcript streams to right panel     │
  │  User may request a Socratic hint      │
  │  User finishes → AI acknowledges       │
  │  Evaluation fires → heatmap reveals    │
  │  Difficulty band updates               │
  │  Band + weak concepts injected back    │
  └────────────────────────────────────────┘
      ↓
  End session (user-triggered or after N turns / time cap)
  → Report: radar chart, per-turn breakdown, delivery metrics
  → Roadmap: day-by-day plan with real resource links
  → Export / share
```

### 3.5 Journey B — Study Mode

```
Enter topic ("transformer attention", "OS process scheduling")
  → Syllabus decomposition into dependency-ordered subtopics
  → Select depth (survey / exam-ready / interview-ready)
  → Drill loop: recall → application → edge case → "teach it back"
  → Same evaluation engine, text-first
  → Mastery map per subtopic + roadmap
```

---

## 4. Scope

### 4.1 In scope (must ship)

- Firebase Auth (Google sign-in only)
- Resume PDF + JD ingestion → structured digest
- Three interviewer personas with distinct rubrics and distinct voices
- Live Room with bidirectional native audio
- Live transcription of both sides
- Post-turn evaluation with span-level heatmap
- Socratic hint system with score accounting
- Adaptive difficulty with visible band changes
- Post-session report with radar chart and per-turn detail
- Delivery metrics (WPM, filler words, pause analysis)
- Study roadmap with grounded resource links
- Study Mode (text-first)

### 4.2 Out of scope (explicitly not building)

- Video / webcam analysis. Eye contact and posture scoring is a tar pit of latency and false signal. Say "future scope" if asked.
- Coding IDE with execution. A shared editor with a judge0-style runner is a whole second project.
- Multi-user or peer mock interviews.
- Payment, plans, or org accounts.
- Native mobile apps. Responsive web only.
- Interview recording playback with synced audio scrubbing. Nice, not necessary.

### 4.3 Deferred (built if time permits, in this order)

1. **Panel mode** — all three personas in one session, handing off between each other. Very high demo value, moderate build cost. This is the first thing to add if you're ahead of schedule.
2. Resume rewrite suggestions derived from interview performance.
3. Spaced-repetition scheduling of the roadmap.
4. Session-over-session progress tracking.

### 4.4 Non-functional requirements

| Requirement | Target | Rationale |
|---|---|---|
| Time to first AI audio after entering Live Room | < 2.5 s | Anything slower reads as broken on stage |
| Round-trip latency, user stops speaking → AI begins speaking | < 1.2 s | This is the whole value of native audio; a pipeline of STT→LLM→TTS lands at 3–4 s and feels dead |
| Transcript delta appearing on screen | < 400 ms behind speech | Creates the "it's listening" effect |
| Post-turn evaluation completion | < 4 s | Slower than the AI's next question is acceptable; slower than 6 s feels stuck |
| Concurrent sessions supported | 10 | Judges may all click at once |
| Session hard cap | 12 minutes | Credit protection |
| Uptime during judging window | 100% | Min instances = 1, no cold starts |

---

## 5. Success Metrics

### 5.1 Demo-day acceptance criteria (binary, testable)

- [ ] A cold-start user completes signup → resume upload → 3-turn voice interview → report → roadmap without a single manual intervention.
- [ ] The AI's first question demonstrably references a specific project from the uploaded resume.
- [ ] A deliberately vague answer produces at least one amber span; a deliberately fabricated claim produces at least one red span.
- [ ] A deliberately excellent answer produces **zero** red spans. *(This is the most important test in this document. Rehearse it.)*
- [ ] The difficulty band indicator visibly changes after two strong or two weak answers.
- [ ] The roadmap contains at least three resource links that resolve to real pages.
- [ ] Total Vertex spend for a full 10-minute session stays under the per-session budget in §21.

### 5.2 Product metrics (post-hackathon framing)

- Turn completion rate (answers over 20 words / questions asked)
- Hint dependency rate per difficulty band
- Score delta between a user's first and third session
- Roadmap item completion (requires the deferred progress-tracking feature)

---

# PART II — PRODUCT SPECIFICATION

## 6. Mode A — Interview Coach

### 6.1 Ingestion: resume + job description

**Input:** one PDF (≤ 10 MB, ≤ 5 pages) and one block of pasted text (≤ 20,000 chars).

**Implementation note that saves you two hours:** do **not** add a PDF text-extraction library. Gemini on Vertex accepts PDF bytes as an inline `Part` with mime type `application/pdf` and reads it multimodally, which means it handles two-column layouts, tables, and design-heavy resumes that would defeat a text extractor. Upload the raw bytes to Cloud Storage, then pass either the GCS URI or the inline base64 to a single `GenerateContent` call with a response schema.

**Output — the Session Digest**, a single structured object:

```json
{
  "candidate": {
    "seniority_estimate": "entry | junior | mid | senior",
    "primary_stack": ["python", "pytorch", "gcp"],
    "claims": [
      {
        "id": "c1",
        "text": "Built an async proxy layer handling 2k req/s",
        "artifact": "project: DataMesh",
        "verifiable_depth": "high | medium | low",
        "probe_angles": [
          "How was backpressure handled?",
          "What was the concurrency primitive?",
          "How was 2k req/s measured?"
        ]
      }
    ],
    "gaps_vs_jd": ["no distributed training experience", "no MLOps tooling named"]
  },
  "role": {
    "title": "ML Engineer",
    "must_haves": ["...as extracted from JD..."],
    "nice_to_haves": ["..."],
    "implied_seniority": "mid",
    "domain_areas": ["classical ML", "feature engineering", "model serving"]
  },
  "interview_plan": [
    {
      "area": "Feature pipeline design",
      "why": "JD demands it; resume claims it; depth unverified",
      "opening_question_seed": "...",
      "target_band": 3
    }
  ]
}
```

The `probe_angles` array is what makes questions feel uncanny. The AI isn't asked to "interview based on this resume" — it's handed pre-computed lines of attack.

**The `interview_plan` is shown to the user before they enter the room, with a checkbox per area.** This is a small feature with an outsized effect: it converts the tool from something happening *to* the user into something they configured, and it gives you a natural beat in the demo to explain what the system extracted.

### 6.2 Persona selection

Presented as three cards. Each card shows the persona's name, one line on what they care about, and — importantly — **what they will punish you for**. Users should be able to choose the one that scares them.

### 6.3 The turn structure

A session is a sequence of turns. Each turn has a lifecycle:

```
QUEUED → ASKING → LISTENING → CLOSING → EVALUATING → SETTLED
```

- **ASKING** — model audio streaming out, visualizer active, right panel locked
- **LISTENING** — mic hot, transcript streaming in, hint button enabled
- **CLOSING** — the user has stopped speaking (VAD-detected or manual "Done"); the model gives a brief verbal acknowledgment and, if appropriate, one follow-up probe
- **EVALUATING** — grader running; a subtle shimmer over the transcript
- **SETTLED** — heatmap revealed, scores visible, band possibly updated

### 6.4 Session termination

Three exits: user clicks "End Interview", the 12-minute cap fires, or the interview plan is exhausted. All three converge on the same finalization job. **Always show a confirmation on manual exit** — a mis-clicked end button mid-demo with no confirmation is a bad afternoon.

---

## 7. Mode B — Study Coach

Study Mode reuses the entire evaluation and roadmap machinery and swaps out ingestion and question generation. It exists because the problem statement says *"a topic **or** job role"* and shipping only the interview half leaves a scoring criterion open.

### 7.1 Syllabus decomposition

Input: a topic string, optionally plus a syllabus paste or a target exam name.

Output: dependency-ordered subtopics with prerequisite edges.

```json
{
  "topic": "Transformer attention",
  "subtopics": [
    {"id": "s1", "name": "Dot-product similarity", "prereqs": [], "depth": 1},
    {"id": "s2", "name": "Scaled dot-product attention", "prereqs": ["s1"], "depth": 2},
    {"id": "s3", "name": "Why divide by sqrt(d_k)", "prereqs": ["s2"], "depth": 3},
    {"id": "s4", "name": "Multi-head attention", "prereqs": ["s2"], "depth": 3},
    {"id": "s5", "name": "KV caching at inference", "prereqs": ["s4"], "depth": 4}
  ]
}
```

Rendering this as a small dependency graph in the UI is cheap and looks considered.

### 7.2 The four question archetypes

Study Mode cycles deliberately rather than asking random questions:

1. **Recall** — "State the formula and name each term."
2. **Application** — "Given a 512-dim embedding and 8 heads, what's the per-head dimension and why does that matter?"
3. **Edge case / failure mode** — "What breaks if you remove the scaling factor?"
4. **Teach-back** — "Explain this to someone who knows linear algebra but not ML." *(This one is the highest-signal question type in the entire product and should be prominent.)*

### 7.3 Mastery tracking

Per subtopic: `unseen | attempted | shaky | solid`. A subtopic reaches `solid` only after a correct teach-back, not merely a correct recall. Render as a progress map over the dependency graph.

### 7.4 Voice in Study Mode

Off by default — text is faster for drilling and cheaper. Offer a "say it out loud" toggle per question, which is exactly where the teach-back archetype belongs.

---

## 8. The Interviewer Panel

Three personas. Each is a bundle of: a system instruction, rubric weights, a question-type distribution, a distinct Live API voice, and an interruption policy.

### 8.1 The Tech Lead

- **Cares about:** implementation detail, complexity analysis, edge cases, what actually happens at runtime
- **Punishes:** hand-waving, buzzwords without mechanism, "it just works"
- **Signature move:** takes any answer and asks "and what happens when that fails?"
- **Rubric weights:** technical accuracy 0.50, depth 0.25, structure 0.15, communication 0.10
- **Question distribution:** 60% implementation drill-down, 25% debugging scenario, 15% complexity/tradeoff
- **Interruption policy:** will cut in if the candidate drifts into unrelated territory for more than ~15 seconds
- **Voice:** lower-pitched, brisk, minimal warmth

### 8.2 The System Architect

- **Cares about:** boundaries, data flow, failure domains, scale, why-not-the-other-thing
- **Punishes:** premature detail, unexamined defaults, no mention of tradeoffs
- **Signature move:** "You picked X. Argue for Y instead, then tell me why you still prefer X."
- **Rubric weights:** technical accuracy 0.30, depth 0.20, structure 0.35, communication 0.15
- **Question distribution:** 50% design-from-requirements, 30% tradeoff interrogation, 20% scale/failure
- **Interruption policy:** patient; lets the candidate build a wrong design and then probes the crack
- **Voice:** measured, even pacing

### 8.3 The Product Manager

- **Cares about:** clarity, user impact, prioritisation, how you behave when you disagree with someone
- **Punishes:** jargon without translation, no user in the story, blaming teammates
- **Signature move:** "Explain that to a customer who is angry and non-technical."
- **Rubric weights:** communication 0.45, structure 0.25, technical accuracy 0.20, depth 0.10
- **Question distribution:** 40% behavioural (STAR-shaped), 35% impact/prioritisation, 25% translate-the-technical
- **Interruption policy:** rarely interrupts; will ask "so what was the outcome?" when the candidate stops at process
- **Voice:** warmer, slightly faster, more conversational

### 8.4 System instruction template

Each persona's instruction is assembled at session start from a template. Structure it as: **identity → what you probe → what you never do → the candidate's digest → the plan → current state**. The last block is the only part that changes mid-session, and it changes by injection (see §17.5).

```
You are {persona.identity}.

You are conducting a {role.title} interview at {role.implied_seniority} level.

WHAT YOU PROBE
{persona.probe_doctrine}

HOW YOU BEHAVE
- Ask exactly one question at a time. Never stack questions.
- Keep every utterance under 60 words. You are on a voice call, not writing.
- Never evaluate, score, or say "good answer" or "that's wrong". A separate
  system handles assessment. You may acknowledge briefly and move on.
- Never supply the answer you are testing for, even if the candidate asks.
- If the candidate is silent for more than 6 seconds, offer to rephrase once.

CANDIDATE BACKGROUND
{digest.candidate}

LINES OF ATTACK (use these; they are pre-computed from the candidate's own claims)
{digest.claims[*].probe_angles}

INTERVIEW PLAN
{digest.interview_plan}

CURRENT STATE
Difficulty band: {band} of 5 — {band_description}
Concepts already proven (do not re-test): {concepts_proven}
Concepts shaky (probe adjacent, not identical): {concepts_shaky}
```

Two lines in there are load-bearing and easy to omit:

- **"Never evaluate or score."** If you don't say this, the model will start giving verbal feedback, which duplicates the grader, contradicts it about half the time, and destroys the illusion that the scoring is rigorous.
- **"Keep every utterance under 60 words."** Native-audio models will happily monologue for ninety seconds. Nothing kills a live demo faster.

---

## 9. The Live Room

### 9.1 Layout

```
┌──────────────────────────────────────────────────────────────────────┐
│  CRUCIBLE          ML Engineer · Tech Lead        Band 3/5   09:42 ⏱ │
├────────────────────────────────┬─────────────────────────────────────┤
│                                │                                     │
│      ╭──────────────╮          │   Q3 · Feature pipeline design      │
│      │   ((( • )))  │          │   ─────────────────────────────     │
│      ╰──────────────╯          │                                     │
│         THE TECH LEAD          │   So the ingestion layer used a     │
│                                │   Kafka topic per source, and we    │
│  "Walk me through how you      │   deduplicated downstream using     │
│   handled backpressure in      │   a bloom filter before the         │
│   that proxy layer."           │   feature store write...            │
│                                │                                     │
│  ┌──────────────────────────┐  │   ▌                                 │
│  │  ◈ Request a hint        │  │                                     │
│  │  ⏎ I'm done answering    │  │   ─────────────────────────────     │
│  │  ⌨ Type instead          │  │   ▮▮▮▮▮▮▮▯▯▯  148 wpm              │
│  │  ⏹ End interview         │  │                                     │
│  └──────────────────────────┘  │                                     │
├────────────────────────────────┴─────────────────────────────────────┤
│  Turn 3 of ~6      ● Listening                     Hints used: 1     │
└──────────────────────────────────────────────────────────────────────┘
```

### 9.2 Left panel — the interviewer

- **Persona identity card.** Name, avatar treatment, and the current difficulty band.
- **The visualizer.** Drive it from actual output audio amplitude (RMS over each PCM chunk), not a decorative loop. Judges notice the difference. Idle state should be a slow breathing pulse, not static — a frozen visualizer reads as a crashed app.
- **The question text.** The AI speaks, but the question also appears as text, because in a noisy demo hall nobody can hear it. Source this from the output transcription stream, not from a separate generation.
- **Control cluster.** Hint, Done, Type-instead, End.

### 9.3 Right panel — the candidate

- **The transcript.** Large type, generous line height. This is the emotional centre of the product — the user watching their own thoughts appear. Do not make it a cramped chat bubble.
- **Interim vs. final text.** Render interim transcription at reduced opacity and finalize it as it stabilises. Cheap to do, and it makes the streaming feel alive.
- **Live pace bar.** Deterministic WPM over a rolling 15-second window. No AI call needed — word count over elapsed audio time.
- **Post-turn heatmap.** The same text, re-rendered with span highlights (§12).

### 9.4 State machine and its visual language

| State | Visualizer | Transcript panel | Mic | Controls |
|---|---|---|---|---|
| `CONNECTING` | slow grey pulse | skeleton | off | all disabled |
| `ASKING` | active, amplitude-driven | dimmed, empty | muted | Done disabled, Hint disabled |
| `LISTENING` | soft idle breathing | live, cursor visible | **hot** | Hint + Done enabled |
| `CLOSING` | active | frozen, final text | muted | disabled |
| `EVALUATING` | idle | shimmer overlay | off | disabled |
| `SETTLED` | idle | heatmap revealed | off | Next enabled |
| `ERROR` | red ring | error card + retry | off | Retry, End |

Every one of these needs a distinct visual signature. The most common failure in voice UIs is that the user cannot tell whether the system is listening, thinking, or dead.

### 9.5 The heatmap reveal

Because grading is post-turn, you get a genuine moment: the transcript sits there in plain text for a beat, then the spans illuminate in sequence, left to right, over roughly 600ms. Stagger the animation. It costs twenty lines of CSS and it is the single most screenshot-able frame in the product.

### 9.6 Accessibility and fallbacks

- **"Type instead"** switches the turn to text input, routed through the identical evaluation path. Required for accessibility and as your demo safety net when the venue mic is bad.
- Keyboard shortcut for Done (Space held to talk is tempting but conflicts with scrolling — use `Ctrl+Enter`).
- Captions are already inherent to the design; ensure contrast ratios on the heatmap colours meet WCAG AA, and never encode a verdict by colour alone — pair each with an icon (`✓ ~ !`).

---

## 10. Adaptive Difficulty Engine

### 10.1 The five bands

| Band | Name | Character of questions | Tolerance |
|---|---|---|---|
| 1 | Orientation | Definitional, "what is / when would you use" | Accepts textbook answers |
| 2 | Application | "Given this situation, what would you do" | Wants a reason attached |
| 3 | Mechanism | "How does that work under the hood" | Wants correct internals |
| 4 | Tradeoff | "Argue against your own choice" | Wants named alternatives and costs |
| 5 | Adversarial | Deliberately underspecified; requires the candidate to surface assumptions | Wants them to push back on the question |

Band 3 is the default entry point for a mid-level role, band 2 for entry-level. Never start at band 1 for a candidate with a real resume — it's insulting, and it wastes the first 90 seconds of a 10-minute demo.

### 10.2 Scoring and the ladder

```
turn_score = Σ (rubric_weight_i × dimension_score_i)   # persona-weighted, 0–10
             − 0.5 × hints_used

rolling = 0.6 × turn_score(n) + 0.4 × turn_score(n−1)
```

Transition rules:

- `rolling ≥ 7.5` for two consecutive turns → **promote** one band
- `rolling ≤ 4.0` for two consecutive turns → **demote** one band
- One band change per two turns maximum (prevents oscillation)
- Never demote below 2 (demoralising, and it makes the demo look easy)
- Never promote above 5

### 10.3 Concept coverage

The engine maintains three sets on the session document:

- `concepts_proven` — demonstrated correctly at band ≥ current. Never re-tested.
- `concepts_shaky` — attempted, partially correct. Re-approached from a different angle, not repeated verbatim.
- `concepts_missing` — named in the plan or the JD but never successfully addressed. **This set is the input to the roadmap.**

Without coverage tracking, an adaptive system asks the same thing three times in different words, which users notice immediately.

### 10.4 Making adaptation visible

Adaptation that the user cannot perceive is worthless in a demo. Three surfacings:

1. Band indicator in the header, which animates on change with a one-line toast: *"Difficulty raised — you've proven the fundamentals."*
2. The persona verbally acknowledges it, once, when promoted: *"Alright, let's go deeper."* (Achieved by injecting a hint into the system state, not by scripting the audio.)
3. The report shows a band-over-time sparkline.

---

## 11. Evaluation and Feedback

### 11.1 Rubric dimensions

Every answer is scored 1–10 on four dimensions. The problem statement requires accuracy and clarity; the other two exist because they're what actually differentiates candidates.

| Dimension | Question it answers | Anchors |
|---|---|---|
| **Technical accuracy** | Is what they said correct? | 10 = precise and complete; 7 = correct with minor imprecision; 4 = partially correct, notable errors; 1 = fundamentally wrong |
| **Communication clarity** | Could a competent listener follow it? | 10 = crisp, well-sequenced, no backtracking; 4 = follows only with effort; 1 = incoherent |
| **Depth** | Did they go past the surface? | 10 = mechanism-level with tradeoffs; 5 = one level below definition; 1 = restated the question |
| **Structure** | Was the answer organised? | 10 = explicit framing then detail then conclusion; 5 = organised but unsignposted; 1 = stream of consciousness |

Include the anchor descriptions **in the evaluator's prompt**. Unanchored 1–10 scales from an LLM cluster tightly around 7 and are useless for adaptation.

### 11.2 The evaluation schema

Use Vertex controlled generation (`responseSchema` with `responseMimeType: application/json`) so this is guaranteed-shape, not parsed-and-prayed.

```json
{
  "turn_id": "t3",
  "question_intent": "Whether the candidate understands backpressure mechanics vs. merely naming a queue",
  "scores": {
    "technical_accuracy": 6,
    "communication_clarity": 8,
    "depth": 5,
    "structure": 7
  },
  "verdict_summary": "Correctly identified the queueing approach and deduplication strategy, but described backpressure as a buffer size rather than a flow-control signal. Never addressed what happens when the consumer stalls.",
  "spans": [
    {
      "excerpt": "deduplicated downstream using a bloom filter",
      "verdict": "validated",
      "concept": "probabilistic deduplication",
      "explanation": "Correct and appropriate choice; acknowledges the false-positive tradeoff implicitly."
    },
    {
      "excerpt": "backpressure was just a bigger buffer",
      "verdict": "incorrect",
      "concept": "backpressure",
      "explanation": "A larger buffer delays the problem; it is not flow control. Backpressure requires signalling the producer to slow down.",
      "correction": "Bounded buffer plus a blocking or rate-limiting signal upstream."
    },
    {
      "excerpt": "handling about 2000 requests per second",
      "verdict": "unsupported",
      "concept": "throughput measurement",
      "explanation": "Figure asserted without describing how it was measured or under what conditions. An interviewer will press on this."
    }
  ],
  "concepts_demonstrated": ["message queue fan-in", "probabilistic deduplication"],
  "concepts_missing": ["flow control / backpressure signalling", "consumer lag monitoring"],
  "ideal_answer_outline": [
    "Bounded queue with explicit capacity",
    "Producer-side signalling on high-water mark",
    "Consumer lag as the observed metric",
    "Load-shedding policy when the signal is ignored"
  ],
  "followup_probe": "What would you have observed in your metrics if the consumer had stalled for thirty seconds?",
  "difficulty_recommendation": "hold"
}
```

Note `followup_probe`. The evaluator generates the sharpest possible next question, and that gets injected into the live session — which means adaptation is driven by the grader, not by the interviewer's improvisation. This is the architectural choice that makes the adaptation actually good.

### 11.3 Which model grades

`gemini-3-flash` for the primary evaluation — it's the reasoning-quality tier you need for span-level judgement. `gemini-3.1-flash-lite` for the cheap high-volume auxiliaries (hint generation, delivery metric labelling, transcript cleanup). Do not use `gemini-2.5-flash`; it is on the deprecation path as of mid-2026.

### 11.4 The report screen

- **Radar chart** across the role's `domain_areas`, scored by aggregating turn scores tagged to each domain. Recharts `RadarChart`. This is the visual the problem statement is implicitly asking for.
- **Band-over-time sparkline.**
- **Per-turn accordion:** question, your answer with heatmap intact, the four scores, the verdict summary, the ideal answer outline, and whether you used a hint.
- **Delivery panel** (§13).
- **Two lists, side by side:** "You proved" and "You need to close". Keep the second one to five items maximum. A list of nineteen weaknesses is not actionable, it's just discouraging.

---

## 12. The Concept Heatmap

### 12.1 The verdict taxonomy

Your original design had two states, green and red. Ship four:

| Verdict | Colour | Icon | Meaning | When the evaluator should use it |
|---|---|---|---|---|
| `validated` | green | ✓ | Correct and substantive | High confidence the claim is right and relevant |
| `incomplete` | amber | ~ | Directionally right, thin | Correct as far as it goes, but the mechanism or the consequence is missing |
| `unsupported` | blue | ? | Asserted without basis | Specific claims — numbers, outcomes, "we scaled to X" — offered with no measurement or reasoning |
| `incorrect` | red | ! | Factually wrong | High confidence the claim is false |

**Why `unsupported` matters:** most of what you were calling "hallucination" in a candidate's answer is not fabrication, it's unbacked assertion. Distinguishing them is more honest and more useful — and it protects you from the demo-killing failure mode of flagging a true statement red because the model couldn't verify it.

**Calibration instruction for the evaluator prompt:** *"Use `incorrect` only when you are confident the statement is false. If you are uncertain whether a claim is true, use `unsupported`. Flagging a correct statement as incorrect is a severe failure."* Say this explicitly. It measurably reduces false reds.

### 12.2 Span anchoring — the implementation trap

Language models are unreliable at character offsets. If you ask for `start_char` and `end_char`, roughly a third of your spans will be off by a few characters and your highlights will land mid-word.

**Solution:** ask for the verbatim `excerpt` string only, then anchor server-side in Go:

1. Exact substring search. Covers ~85% of cases.
2. Case- and punctuation-insensitive normalised search. Covers most of the rest.
3. Token-level fuzzy match (Levenshtein over a sliding window, accept ≤ 15% distance).
4. If all three fail: **drop the span silently** and keep the concept in `concepts_missing`. A missing highlight is invisible; a wrong highlight is a bug the judge sees.

Log the drop rate. If it exceeds 20%, the evaluator is paraphrasing rather than quoting — tighten the prompt with *"excerpt must be copied character-for-character from the transcript."*

### 12.3 Interaction

Hover (desktop) or tap (mobile) a span → popover with the concept name, the explanation, and — for `incorrect` and `incomplete` — the correction. Keep the popover under forty words. Anything longer belongs in the report.

### 12.4 A note on transcript quality

The heatmap grades the *transcript*, and the transcript is imperfect. A speech model that hears "bloom filter" as "blue filter" will produce a red flag against a correct answer. Mitigations:

- Pass a **domain vocabulary hint** into the evaluator prompt: the resume's `primary_stack` plus the JD's technical nouns. Instruct the evaluator to treat near-miss transcriptions of these terms charitably.
- Add an explicit line: *"The transcript is from speech recognition and may contain phonetic errors. If a term appears to be a mis-transcription of a plausible technical term, interpret it charitably and do not penalise it."*

This single instruction will resolve most of your false positives.

---

## 13. Delivery and Non-Verbal Metrics

### 13.1 The filler-word problem — read this before building

Google's speech recognition **normalises disfluencies out of the transcript.** "Um", "uh", and false starts are removed as noise. This is correct behaviour for dictation and fatal for your feature. If you count fillers by regex over the Live API transcript, you will ship a counter that always reads zero, and you probably won't notice until demo day.

Three viable paths:

| Path | How | Verdict |
|---|---|---|
| **A. Post-turn audio analysis** | Send the answer's raw audio to `gemini-3-flash` with a structured schema asking for filler instances, pace, and hesitation | **Recommended.** One extra call per turn, works today, no second pipeline, and the model hears things a transcript can't |
| B. Parallel Speech-to-Text v2 | Fork the audio stream to Chirp with word time offsets, count from raw tokens | Accurate timing, but a whole second audio pipeline and double the audio cost |
| C. Client-side VAD | Web Audio energy analysis for pauses and pace only, no filler detection | Free, useful for the live pace bar, insufficient alone |

**Build A for the report and C for the live pace bar.** They serve different purposes and don't conflict.

### 13.2 Deterministic vs. inferred metrics

Compute what you can compute. Never ask a model for arithmetic you can do in Go.

| Metric | Source | Method |
|---|---|---|
| Words per minute | Deterministic | `word_count / (audio_duration_ms / 60000)` |
| Total speaking time | Deterministic | Sum of audio frames received during LISTENING |
| Longest silence | Deterministic | Gap between voiced frames, client-side VAD |
| Response latency | Deterministic | Time from ASKING end to first voiced frame |
| Filler count and instances | Inferred (path A) | Gemini over answer audio |
| Hesitation score | Inferred | Gemini, informed by pause distribution |
| Vocal confidence / monotony | Inferred | Gemini over audio |

### 13.3 The pace dial

Bands: `< 110 wpm` Hesitant · `110–160` Optimal · `160–190` Rushed · `> 190` Too fast.

Present as a dial with the optimal band shaded. Include the caveat that optimal pace is context- and language-dependent — a system-design walkthrough should be slower than a behavioural story.

### 13.4 What not to say

Delivery feedback is where a coaching tool most easily becomes cruel. Two rules:

- Report counts and observations, not character judgements. *"You said 'um' 14 times in 90 seconds, mostly right after each question"* — not *"you sound unconfident."*
- Always pair an observation with a drill. *"Try a deliberate two-second pause instead of a filler when you need thinking time. Re-run this question and watch the counter."*

---

## 14. The Study Roadmap

### 14.1 Aggregation

At session end, collect every `concepts_missing` entry across turns. Then:

1. **Cluster** near-duplicates ("backpressure", "flow control", "producer throttling" → one cluster).
2. **Score** each cluster: `frequency × severity × jd_relevance`, where `jd_relevance` is 2.0 if the concept appears in the JD's `must_haves`, 1.3 for `nice_to_haves`, 1.0 otherwise.
3. **Rank** and take the top N, where N ≈ 1.5 × available days.
4. **Order** by prerequisite dependency, not by score. Learning is ordered even when priorities aren't.

### 14.2 Resource grounding

Generate resources with **Grounding with Google Search** enabled on the Vertex call. Ungrounded, the model will invent plausible-looking documentation URLs, and a judge who clicks a 404 will remember it.

Constrain with a domain allowlist in the prompt: official project documentation, `arxiv.org`, well-known university course pages, and a small set of reputable technical publishers. Explicitly exclude content-farm and answer-scraping domains.

Google Search grounding on Vertex includes a monthly free allowance (5,000 prompts as of mid-2026, then billed per thousand queries), which is far beyond hackathon needs. Budget one grounded call per roadmap, not one per item — batch the resource lookup.

### 14.3 Roadmap schema

```json
{
  "session_id": "...",
  "horizon_days": 11,
  "summary": "Your fundamentals are solid. The gap is depth on distributed-system failure modes, which is exactly what a mid-level ML Engineer loop will hit.",
  "days": [
    {
      "day": 1,
      "focus_concept": "Backpressure and flow control",
      "why_this_matters": "You lost the most points here, and the JD names streaming pipelines as a must-have.",
      "estimated_minutes": 75,
      "resources": [
        {"title": "Reactive Streams specification — flow control section", "url": "...", "type": "spec", "minutes": 25},
        {"title": "Kafka consumer lag monitoring guide", "url": "...", "type": "docs", "minutes": 20}
      ],
      "practice_task": "Take your DataMesh proxy and write down, in four sentences, what happens to the producer when the consumer stalls for 30 seconds. If you can't, that's the gap.",
      "self_check": "Explain the difference between a bounded buffer and backpressure without using the word 'buffer' twice."
    }
  ],
  "retest_plan": {
    "after_day": 4,
    "focus_areas": ["flow control", "consumer lag"],
    "recommended_persona": "tech_lead",
    "recommended_band": 4
  }
}
```

The `retest_plan` closes the loop: the roadmap ends by pointing back into the product with a specific, pre-configured next session. Build the button for it.

---

# PART III — TECHNICAL ARCHITECTURE

## 15. System Overview

### 15.1 Component diagram

```
┌───────────────────────────────────────────────────────────────────────┐
│  BROWSER  (React + TypeScript + Vite + Tailwind)                      │
│                                                                       │
│   AudioWorklet capture ──► PCM16 @16kHz, 20ms frames                  │
│   AudioWorklet playback ◄── PCM16 @24kHz, ring buffer                 │
│   Firebase Auth SDK  ──► ID token                                     │
│   WebSocket client   ◄─► binary audio + JSON control frames           │
└───────────────┬───────────────────────────────────────────────────────┘
                │  WSS + HTTPS
                ▼
┌───────────────────────────────────────────────────────────────────────┐
│  CLOUD RUN  ·  single Go service  ·  min-instances=1                  │
│                                                                       │
│  ┌─────────────┐  ┌──────────────────┐  ┌──────────────────────────┐  │
│  │  httpapi    │  │   liveproxy      │  │   workers                │  │
│  │  REST       │  │   1 goroutine    │  │   buffered chan +        │  │
│  │  sessions,  │  │   pair / session │  │   worker pool            │  │
│  │  uploads,   │  │   client WS ◄─►  │  │   · evaluate turn        │  │
│  │  reports    │  │   Vertex bidi WS │  │   · delivery metrics     │  │
│  └──────┬──────┘  └────────┬─────────┘  │   · finalize report      │  │
│         │                  │            │   · build roadmap        │  │
│  ┌──────┴──────────────────┴────────────┴──────────────────────┐     │
│  │  authn (Firebase ID token) · session store · vertex client  │     │
│  └──────┬────────────────────────────────┬─────────────────────┘     │
└─────────┼────────────────────────────────┼───────────────────────────┘
          │                                │  ADC / service account
          ▼                                ▼
┌──────────────────────┐   ┌────────────────────────────────────────────┐
│  Firestore           │   │  VERTEX AI  (region: us-central1)          │
│  · sessions          │   │                                            │
│  · turns             │   │  gemini-live-2.5-flash-native-audio        │
│  · evaluations       │   │     └─ bidi WS, speech-to-speech           │
│  · reports           │   │  gemini-3-flash                            │
│  · roadmaps          │   │     └─ digest, evaluation, delivery,       │
│                      │   │        roadmap (+ Search grounding)        │
│  Cloud Storage       │   │  gemini-3.1-flash-lite                     │
│  · resumes/          │   │     └─ hints, labels, cheap fan-out        │
│  · audio/{turn}.wav  │   │                                            │
└──────────────────────┘   └────────────────────────────────────────────┘
```

### 15.2 Why a single Go service

You asked for Go alone if possible, and it is possible — I checked. The `google.golang.org/genai` SDK exposes the Live bidi surface with a Vertex backend, connecting to `ws/google.cloud.aiplatform.{version}.LlmBidiService/BidiGenerateContent`. Nothing in this design needs Python:

- **PDF parsing?** Not needed — Gemini reads the PDF natively.
- **Audio processing?** Framing and resampling happen in the browser's AudioWorklet; Go just relays bytes.
- **Charts?** Client-side, Recharts.
- **ML?** All of it is API calls.

Go's concurrency model is genuinely the right tool here rather than a flourish: each session is two goroutines and a channel, the evaluation workers are a buffered channel and a worker pool, and you get all of it without an external queue, a second runtime, or a container to coordinate.

**Tradeoff, stated explicitly:** the Go SDK's Live surface is newer than the Python one and parts of it are marked preview. Some example code and community answers you'll find will be Python. Budget an extra hour for the first Live connection working end-to-end, and keep the fallback in your pocket: the bidi endpoint is a plain WebSocket with a bearer token and JSON frames, so `gorilla/websocket` plus `golang.org/x/oauth2/google` gets you there by hand if the SDK fights you. **Prototype the Live connection first, before you build anything else.** If it isn't working within ninety minutes, that's your signal to bring in a single Python sidecar for the live proxy only and keep the rest in Go.

### 15.3 Frontend stack

React 19 + TypeScript + Vite. Tailwind for layout. Recharts for the radar and sparkline. Zustand for session state — the state machine in §9.4 wants a single store, and Redux is overkill for a hackathon. No component library; the Live Room is custom anyway and shadcn-flavoured defaults are exactly the generic look you don't want here.

---

## 16. Vertex AI Integration Layer

### 16.1 Authentication — the constraint that shaped this architecture

Vertex AI authenticates with an **OAuth2 bearer token minted from a service account**, not an API key. This has a consequence worth stating plainly, because it invalidates the obvious architecture:

> **The browser cannot connect to the Vertex Live API directly.** A service account key placed in frontend code is a credential leak that grants access to your entire GCP project. There is no safe version of this.

So the backend relay is not a performance optimisation — it is structurally mandatory. Your original instinct to build a gateway was correct for a reason you hadn't identified yet.

**Credential handling by environment:**

| Environment | Mechanism |
|---|---|
| Local development | `GOOGLE_APPLICATION_CREDENTIALS` pointing at the service account key JSON, gitignored, never committed |
| Cloud Run | **No key file.** Attach the service account to the service and let Application Default Credentials resolve it. This is more secure and less work |
| If a key file is truly needed in prod | Secret Manager, mounted as a volume, never baked into the image |

The Go SDK picks up ADC automatically:

```go
client, err := genai.NewClient(ctx, &genai.ClientConfig{
    Project:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
    Location: "us-central1",
    Backend:  genai.BackendVertexAI,
})
```

Required IAM on the service account: `roles/aiplatform.user`, `roles/datastore.user`, `roles/storage.objectAdmin` scoped to the one bucket. Nothing else. Do not use the default compute service account with Editor.

**Verification step for your own peace of mind:** after wiring this up, check the Vertex AI billing/usage dashboard in the console and confirm requests are appearing there. That's your proof the credits are being drawn from Vertex and not from a stray Gemini API key in an env file.

### 16.2 Region pinning

**Live API models are not available in the `global` location on Vertex.** Pin an explicit region — `us-central1` is the safest default for model availability. Set it once as a constant, not per-call, and make sure the same region is used for both the Live connection and the text calls to avoid confusing latency asymmetry.

### 16.3 Model routing table

| Job | Model | Why | Config notes |
|---|---|---|---|
| Live interview conversation | `gemini-live-2.5-flash-native-audio` | GA on Vertex, 30 HD voices, native speech-to-speech | `responseModalities: ["AUDIO"]`, input + output transcription on |
| Resume + JD digest | `gemini-3-flash` | Multimodal PDF reading, strong structured extraction | `responseSchema`, PDF as inline or GCS part |
| Turn evaluation | `gemini-3-flash` | Span-level judgement needs the reasoning tier | `responseSchema`, temperature 0.2 |
| Delivery metrics | `gemini-3-flash` | Needs to hear the audio, not read a transcript | audio part + `responseSchema` |
| Hint generation | `gemini-3.1-flash-lite` | Cheap, low-latency, simple task | temperature 0.6 |
| Syllabus decomposition | `gemini-3-flash` | Dependency ordering benefits from reasoning | `responseSchema` |
| Roadmap + resources | `gemini-3-flash` | Needs Search grounding | `tools: [{googleSearch: {}}]` |

**Fallback path:** there's a newer `gemini-3.1-flash-live-preview` audio-to-audio model. It's preview, and proactive audio and affective dialogue aren't supported on it yet. Try it once during setup and A/B the voice quality; if it feels better, use it, but do not let a preview model be your demo dependency without a tested fallback. Keep the model ID in config, not inline.

### 16.4 Voice selection

Assign a distinct voice per persona from the native-audio voice set (30 HD voices across 24 languages are available on the native-audio model). Pick them by ear during setup, not from the names — write the three chosen IDs into the persona config. Three interviewers who sound identical undercuts the entire multi-agent premise.

### 16.5 Structured output discipline

Every non-conversational call uses `responseMimeType: "application/json"` with an explicit `responseSchema`. Reasons: guaranteed parse, no markdown-fence stripping, and Go can unmarshal straight into a typed struct.

Two rules that will save you debugging time:

- **Keep schemas flat where possible.** Deeply nested schemas increase malformed-output rates and are harder to debug at 3 a.m.
- **Validate after unmarshalling anyway.** Schema compliance doesn't guarantee semantic sanity — clamp scores to 1–10, drop spans with empty excerpts, cap array lengths.

### 16.6 Retry and degradation

```
Transient (429, 503, timeout)
  → exponential backoff, 3 attempts, jittered, 250ms base

Evaluation call fails permanently
  → mark turn as UNGRADED, show "couldn't grade this one" inline,
    interview continues. NEVER block the conversation on the grader.

Live connection drops mid-session
  → attempt one silent reconnect with prior context replayed via
    SendClientContent; if that fails, surface "reconnecting…" then
    offer to switch to text mode with the session intact

Roadmap grounding fails
  → generate the roadmap ungrounded but omit the resource links,
    with a note. A roadmap with no links beats no roadmap
```

The principle: **the conversation is the product and everything else is enrichment.** Every failure mode degrades toward "the interview keeps working."

---

## 17. The Real-Time Audio Pipeline

### 17.1 Capture (browser)

```
getUserMedia({audio: {
  channelCount: 1,
  echoCancellation: true,     // essential — the AI's voice is on your speakers
  noiseSuppression: true,
  autoGainControl: true
}})
  → AudioContext(sampleRate: 16000)
  → AudioWorkletNode (custom processor, not the deprecated ScriptProcessor)
  → Float32 → Int16 PCM conversion
  → 320-sample frames (20ms @16kHz)
  → WebSocket.send(ArrayBuffer)
```

**Echo cancellation is not optional.** Without it, the AI hears itself through your speakers, interprets it as user speech, and the session degrades into the model interrupting itself. This will happen to you at least once. Use headphones for the demo regardless.

### 17.2 Playback (browser)

Output arrives as PCM16 at 24kHz (note the sample-rate asymmetry — 16k in, 24k out). Feed a ring buffer consumed by a playback AudioWorklet. Do not create an `AudioBufferSourceNode` per chunk; you'll get clicks at every boundary.

Keep 2–3 chunks of jitter buffer. More adds perceptible latency, less produces dropouts on flaky wifi. Track buffer underruns as a metric — it's your best early warning of a network problem before the demo.

### 17.3 Relay (Go)

Per session, two goroutines and a shared context:

```go
// upstream: client → Vertex
for {
    msgType, data, err := clientConn.ReadMessage()
    if msgType == websocket.BinaryMessage {
        liveSession.SendRealtimeInput(audioBlob(data))
    } else {
        handleControlFrame(data)   // hint, done, end
    }
}

// downstream: Vertex → client
for {
    msg, err := liveSession.Receive()
    switch {
    case msg.ServerContent.ModelTurn != nil:
        forwardAudioChunks(clientConn, msg)          // binary frames
    case msg.ServerContent.InputTranscription != nil:
        forwardJSON(clientConn, transcriptDelta{Side: "user", ...})
        appendToTurnBuffer(...)
    case msg.ServerContent.OutputTranscription != nil:
        forwardJSON(clientConn, transcriptDelta{Side: "ai", ...})
    case msg.ServerContent.TurnComplete:
        closeTurnAndDispatchEvaluation()
    }
}
```

Note that a single server event can now carry multiple content parts simultaneously — audio chunks and transcript together — so handle the message as a bag of parts rather than a tagged union with one field set.

### 17.4 Turn boundary detection

This is subtler than it looks with speech-to-speech, because there is no request/response boundary to hook. The user's answer is complete when one of these fires:

1. The Live API's VAD signals end of user activity and the model begins its turn (the common case)
2. The user clicks **Done** (explicit, and the one to rely on for the demo)
3. 10 seconds of silence with a non-empty transcript buffer

On any of these: snapshot the accumulated user transcript, flush the accumulated audio frames to a WAV in Cloud Storage, write the turn document to Firestore, and push an evaluation job onto the worker channel. **Do not await it.**

### 17.5 The Injection Loop — how adaptation actually reaches the conversation

The central design problem of a speech-to-speech architecture: you don't get to compose each prompt. There's one long-lived session, and the model decides what to say next on its own. So how does a grade computed *after* turn 3 change the question asked *in* turn 4?

Answer: you inject it as context between turns.

```
t=0.0s   User stops speaking
t=0.3s   Live model begins a brief acknowledgment  ("Right, okay.")
t=0.3s   [parallel] Evaluation job dispatched to worker pool
t=3.1s   Evaluation returns
t=3.1s   Heatmap reveals in the UI
t=3.2s   Difficulty engine updates band and coverage sets
t=3.3s   Go injects a system-role turn into the live session:
           SendClientContent(turns: [{
             role: "user",
             parts: [{text: "[COACH STATE UPDATE — not from the candidate,
                      do not read this aloud, do not acknowledge it]
                      Difficulty band is now 4. Concepts proven: X, Y.
                      Concepts shaky: backpressure. Do not re-test X or Y.
                      Ask this next: 'What would you have observed in your
                      metrics if the consumer had stalled for thirty
                      seconds?'"}]
           }])
t=3.5s   Model asks the injected question, in its own voice and phrasing
```

Three things make this work:

- **The acknowledgment buys you the time.** The model's brief "right, okay" covers the ~3s grading window, so the pause never feels like lag. Instruct the persona to acknowledge briefly and *wait* rather than immediately generating its own next question — this is a line in the system instruction: *"After the candidate finishes, acknowledge briefly in under 8 words, then wait for the coach state update before asking your next question."*
- **The `followup_probe` from the grader becomes the next question.** The grader saw exactly where the answer thinned out, so its question is sharper than anything the interviewer would improvise. This is where "adaptive" stops being a checkbox.
- **The bracketed framing keeps the injection invisible.** Prefix every injected turn with the do-not-read-aloud marker. Test this specifically — the failure mode is the interviewer reading your internal state out loud, which is both funny and fatal on stage.

### 17.6 Interruption handling

Native audio supports interruption: if the user starts talking while the model is speaking, the model stops. Surface this in the UI by transitioning `ASKING → LISTENING` on the interruption signal, and discard queued playback chunks immediately rather than letting the buffer drain. A model that keeps talking for two seconds after being interrupted feels broken.

---

## 18. Data Model

Firestore, document-oriented, denormalised for read speed. No joins, no migrations, no schema ceremony.

```
users/{uid}
  displayName, email, createdAt, sessionCount

sessions/{sessionId}
  uid, mode: "interview" | "study"
  status: "configuring" | "live" | "evaluating" | "complete" | "abandoned"
  persona: "tech_lead" | "architect" | "pm"
  createdAt, startedAt, endedAt, durationMs
  difficultyBand: 3
  bandHistory: [{turnIndex, band, reason}]
  digest: { ...Session Digest from §6.1... }
  coverage: {
    proven: ["message queue fan-in", ...],
    shaky:  ["backpressure"],
    missing:["consumer lag monitoring", ...]
  }
  liveSessionMeta: { model, voice, region, resumeCount }
  resumeGcsUri, jdText

sessions/{sessionId}/turns/{turnId}
  index: 3
  questionText, questionConcepts: [...], questionBand: 3
  askedAt, answerStartedAt, answerEndedAt
  userTranscript, userTranscriptFinal: bool
  inputMode: "voice" | "text"
  audioGcsUri, audioDurationMs
  hintsUsed: 1, hints: [{text, requestedAt}]
  evaluation: { ...schema from §11.2... }     // embedded, not a subcollection
  delivery:   { ...schema from §13.2... }
  gradingStatus: "pending" | "complete" | "failed"

sessions/{sessionId}/report/summary          (single doc)
  aggregateScores: {technical, communication, depth, structure}
  domainScores: [{domain, score, turnCount}]   // radar chart source
  deliveryAggregate: {wpm, fillerTotal, fillerPerMinute, longestPauseMs}
  strengths: [...], gaps: [...]
  generatedAt

sessions/{sessionId}/roadmap/plan             (single doc)
  { ...schema from §14.3... }
```

**Cloud Storage layout:**

```
gs://{bucket}/resumes/{uid}/{sessionId}.pdf
gs://{bucket}/audio/{sessionId}/{turnId}.wav
```

**Design notes:**

- Evaluation is **embedded in the turn document**, not a subcollection. One read gets you everything to render a turn. Subcollections here would buy you nothing and cost you a round trip per turn.
- `bandHistory` is denormalised on the session so the sparkline needs no aggregation query.
- Set a **lifecycle rule on the audio prefix**: delete after 7 days. Audio is the bulk of your storage and you don't need it after the report exists. Also the right call on privacy.
- Firestore security rules: a user reads and writes only documents where `uid == request.auth.uid`. Write these on day one, not day three — retrofitting rules is miserable.

---

## 19. API Contract

### 19.1 REST

All endpoints require `Authorization: Bearer <firebase-id-token>`. Base path `/v1`.

| Method | Path | Purpose | Notes |
|---|---|---|---|
| `POST` | `/sessions` | Create session | Body: `{mode, persona?, topic?}` → `{sessionId, status}` |
| `POST` | `/sessions/{id}/resume` | Upload resume | `multipart/form-data`, PDF ≤10MB → `{gcsUri}` |
| `POST` | `/sessions/{id}/jd` | Attach JD text | Body: `{text}` |
| `POST` | `/sessions/{id}/digest` | Run ingestion | Sync, ~4–8s → full Session Digest |
| `PATCH` | `/sessions/{id}/plan` | Edit interview plan | Body: `{droppedAreaIds: []}` |
| `GET` | `/sessions/{id}` | Session state | Poll target for the configuring screen |
| `POST` | `/sessions/{id}/turns/{turnId}/hint` | Request hint | → `{hintText, penaltyApplied}` |
| `POST` | `/sessions/{id}/turns/{turnId}/text-answer` | Submit typed answer | Body: `{text}` → dispatches evaluation |
| `POST` | `/sessions/{id}/end` | End session | Triggers finalization; returns immediately |
| `GET` | `/sessions/{id}/report` | Fetch report | 202 with `{status: "generating"}` if not ready |
| `GET` | `/sessions/{id}/roadmap` | Fetch roadmap | Same 202 pattern |
| `GET` | `/sessions` | List user's sessions | Paginated, for history |
| `GET` | `/healthz` | Liveness | Unauthenticated |

### 19.2 WebSocket

**Endpoint:** `WSS /v1/sessions/{id}/live?token=<firebase-id-token>`

Token in the query string because browsers can't set headers on WebSocket handshakes. Mitigate the exposure: short-lived tokens, and never log full URLs.

**Client → Server**

| Frame type | Format | Payload |
|---|---|---|
| Audio | Binary | Raw PCM16 @16kHz mono, 20ms frames |
| Control | Text (JSON) | `{"type":"start_turn"}` |
| | | `{"type":"end_turn"}` |
| | | `{"type":"request_hint"}` |
| | | `{"type":"barge_in"}` |
| | | `{"type":"ping","t":1234567890}` |

**Server → Client**

| Frame type | Format | Payload |
|---|---|---|
| Audio | Binary | PCM16 @24kHz, prefixed with a 4-byte sequence number |
| Transcript | Text | `{"type":"transcript","side":"user"\|"ai","text":"...","final":false}` |
| State | Text | `{"type":"state","state":"LISTENING","turnIndex":3}` |
| Question | Text | `{"type":"question","text":"...","concepts":[...],"band":3}` |
| Evaluation | Text | `{"type":"evaluation","turnId":"t3","payload":{...}}` |
| Delivery | Text | `{"type":"delivery","turnId":"t3","payload":{...}}` |
| Band change | Text | `{"type":"band","from":3,"to":4,"reason":"..."}` |
| Hint | Text | `{"type":"hint","text":"...","penalty":0.5}` |
| Error | Text | `{"type":"error","code":"...","recoverable":true,"message":"..."}` |
| Pong | Text | `{"type":"pong","t":1234567890}` |

**Keepalive:** client pings every 20 seconds. Cloud Run will close idle connections, and a demo that dies during a thoughtful pause is a demo that dies.

---

## 20. Deployment

### 20.1 Cloud Run configuration

| Setting | Value | Why |
|---|---|---|
| Request timeout | 3600s | Default 300s kills WebSockets mid-session |
| Session affinity | **enabled** | Without it, a reconnect can land on a different instance with no session state |
| Min instances | **1** | Cold start during judging is unforgivable. Costs a few dollars |
| Max instances | 5 | Credit protection |
| CPU | 2 vCPU, always allocated | "CPU only during request" throttles your background workers |
| Memory | 2 GiB | Audio buffers plus Go runtime |
| Concurrency | 20 | Each session holds a WebSocket pair; don't oversubscribe |
| Ingress | All | |

Note that Cloud Run caps WebSocket connections at 60 minutes regardless of timeout settings. Your 12-minute session cap sits comfortably inside that, but handle the close gracefully anyway.

### 20.2 Frontend hosting

Firebase Hosting, with a rewrite for `/v1/**` to the Cloud Run service. Single origin, no CORS configuration, no preflight surprises.

### 20.3 Configuration

Everything through environment variables. **Model IDs in config, never inline** — you will want to swap a model at 2 a.m.

```
GOOGLE_CLOUD_PROJECT=...
VERTEX_LOCATION=us-central1
MODEL_LIVE=gemini-live-2.5-flash-native-audio
MODEL_REASONING=gemini-3-flash
MODEL_CHEAP=gemini-3.1-flash-lite
GCS_BUCKET=...
SESSION_MAX_DURATION_SEC=720
SESSION_IDLE_TIMEOUT_SEC=90
MAX_HINTS_PER_TURN=2
MAX_CONCURRENT_SESSIONS=10
DAILY_SESSION_CAP_PER_USER=5
```

### 20.4 Observability

You have no time for a real observability stack, so instrument the four things that will actually break:

1. **Live connection establishment** — success/failure with error codes. Your highest-risk dependency.
2. **Turn-boundary latency** — user stops speaking → AI starts speaking. Your headline metric.
3. **Evaluation duration and failure rate** — per model call.
4. **Span anchoring drop rate** (§12.2) — silent quality degradation you'd otherwise never notice.

Structured JSON logs to stdout; Cloud Logging picks them up for free. Skip tracing.

### 20.5 CI

GitHub Actions: on push to `main`, `go vet` + `go test ./...` + `docker build` + deploy to Cloud Run. Twenty minutes to set up, and it removes an entire category of demo-day mistake — the one where you deploy by hand from a dirty working tree.

---

## 21. Cost Model and Credit Guardrails

You have Vertex credits, which is a budget, not immunity. Native-audio inference is the most expensive thing in this system by a wide margin: audio tokens cost substantially more than text tokens, and a live session consumes them continuously in both directions.

### 21.1 Where the money goes, per 10-minute interview session

| Component | Volume | Relative cost |
|---|---|---|
| Live audio in (user speech + silence) | ~10 min continuous | **High** |
| Live audio out (AI speech) | ~2–3 min | **High** |
| Live input/output transcription | included | Low |
| Digest call (PDF + JD) | 1 call, ~8k tokens in | Low |
| Turn evaluations | ~6 calls, ~2k in / ~1.5k out | Low-moderate |
| Delivery metrics | ~6 calls with audio | **Moderate** |
| Hints | ~2 calls, flash-lite | Negligible |
| Roadmap + grounding | 1 grounded call | Low |

The live connection is the cost centre. **Everything else is rounding error.** Optimise accordingly — don't waste time micro-tuning your evaluator's token count while leaving sessions running unbounded.

### 21.2 Guardrails to implement on day one

1. **Hard session cap, 12 minutes.** Server-enforced, not client. Warn at 10.
2. **Idle timeout, 90 seconds** of no user audio → close the Live connection, keep the session resumable. This is the single most important guardrail: a forgotten open tab is a slow credit leak.
3. **Daily session cap per user, 5.** Prevents a single enthusiastic tester from burning the demo budget.
4. **Concurrent session cap, 10.** Server-side counter; queue or refuse beyond it.
5. **Skip evaluation on trivial turns** — under 15 words, mark ungraded and move on.
6. **Flash-lite for everything that isn't span-level judgement.**
7. **Batch the roadmap's resource lookup** into one grounded call, not one per day.
8. **Close the Live connection the instant a session ends.** Don't rely on garbage collection or the client's unload handler; do it in the `/end` handler and in a deferred cleanup on the WebSocket goroutine.
9. **Set a GCP budget alert** at 50% and 80% of your credit allocation. Two minutes of setup, and it's the difference between noticing on Tuesday and noticing after the deadline.

### 21.3 A note on measurement

Log token usage from every response's `usageMetadata` into a single `usage/{date}` Firestore document, incremented per call, broken down by model. You'll want the real numbers rather than these estimates, both to tune the guardrails and because "here's our actual per-session unit economics" is a genuinely strong answer to a judge's question about viability.

---

# PART IV — EXECUTION

## 22. Build Phases and the Cut Line

Ordered so that the riskiest thing is proven first and every phase ends with something demonstrable.

### Phase 0 — Foundations *(target: 2 hours)*

- GCP project, enable Vertex AI, Firestore, Cloud Storage APIs
- Service account, IAM roles, key JSON for local dev
- Go module, Cloud Run deploy of a `/healthz` stub, CI pipeline
- React app skeleton, Firebase Auth wired, sign-in working
- **Exit criteria:** deployed URL, you can sign in, `/healthz` returns 200

### Phase 1 — The Live Spike *(target: 3 hours)* ⚠️ **HIGHEST RISK — DO THIS FIRST**

- Go connects to the Vertex Live API and receives audio for a text prompt
- Browser captures mic → PCM → WebSocket → Go → Vertex
- Vertex audio → Go → browser → audible playback
- Input and output transcription rendering as text
- **Exit criteria:** you have an unstructured spoken conversation with Gemini through your own stack.

**This phase decides the project.** Everything downstream assumes it works. If you're not there in three hours, stop and reassess — switch the live proxy to Python, or fall back to the STT→text→TTS pipeline (which is less impressive but a known quantity). Do not build UI while this is unproven.

### Phase 2 — Configuration and personas *(target: 3 hours)*

- Resume upload to GCS, digest call with `responseSchema`
- JD paste, plan generation, plan review screen
- Three persona configs with system instruction assembly
- Three distinct voices selected and wired
- Session creation and Firestore persistence
- **Exit criteria:** the AI's first spoken question references a specific project from an uploaded resume.

### Phase 3 — Evaluation, heatmap, adaptation *(target: 4 hours)*

- Turn boundary detection and turn persistence with audio to GCS
- Evaluation worker with the full schema
- Span anchoring in Go with the three-tier fallback
- Heatmap rendering with the staggered reveal
- Difficulty engine and the Injection Loop
- Hint endpoint with penalty accounting
- **Exit criteria:** the demo path in §24 works end to end.

### ═══ CUT LINE ═══

**Everything above ships or you don't demo. Everything below is upside.** If you reach this line with hours to spare, resist starting Phase 5 polish before Phase 4 exists — a working roadmap is worth more than a prettier Live Room, because the roadmap is what the problem statement asks for by name.

### Phase 4 — Report and roadmap *(target: 3 hours)*

- Finalization job: aggregate scores, domain mapping
- Delivery metrics via the post-turn audio call
- Report screen: radar, sparkline, per-turn accordion, delivery panel
- Roadmap generation with Search grounding
- Retest button
- **Exit criteria:** a completed session produces a report and a roadmap with working links.

### Phase 5 — Study Mode *(target: 2 hours)*

- Topic ingestion, syllabus decomposition, dependency graph render
- Four-archetype drill loop, text-first
- Mastery map
- **Exit criteria:** the compliance matrix in §2 has no gaps.

### Phase 6 — Polish and hardening *(target: remaining time)*

- Error states and empty states for every screen
- Loading states that explain what's happening ("Reading your resume…" beats a spinner)
- The visualizer, properly amplitude-driven
- Mobile responsive pass on the report at minimum
- Rehearse the demo five times end to end
- Record a backup video of a successful run

---

## 23. Risk Register

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Go SDK's Live surface is preview and fights you | Medium | **Critical** | Phase 1 spike before anything else; fallback to hand-rolled `gorilla/websocket` + `oauth2/google`, or a single Python sidecar for the proxy only |
| R2 | Echo — AI hears itself, interrupts itself | **High** | High | `echoCancellation: true`, headphones for the demo, tune interruption sensitivity |
| R3 | Venue wifi degrades the audio stream | Medium | **Critical** | Recorded backup video; "replay a saved session" mode reading a fixture from Firestore; text-mode fallback path |
| R4 | Cloud Run closes the WebSocket mid-demo | Medium | High | Timeout 3600s, session affinity, 20s keepalive ping, min-instances 1 |
| R5 | Evaluator flags a correct answer red | Medium | **High** (credibility) | Four-verdict taxonomy, explicit calibration instruction, charitable-transcription instruction, rehearse the excellent-answer test in §5.1 |
| R6 | Span anchoring produces mid-word highlights | High | Medium | Three-tier anchoring, silent drop on failure, monitor drop rate |
| R7 | Filler-word counter always reads zero | **High if unaddressed** | Medium | §13.1 — post-turn audio analysis, not transcript regex. Test with a deliberately disfluent answer early |
| R8 | Credits exhausted before demo day | Medium | **Critical** | All nine guardrails in §21.2, budget alerts at 50/80%, usage logging from day one |
| R9 | Interviewer reads the injected coach state aloud | Medium | High | Bracketed do-not-read marker, explicit system instruction, test specifically |
| R10 | Interviewer monologues for 90 seconds | High | Medium | 60-word utterance cap in the system instruction; verify it holds under band 5 |
| R11 | Live model unavailable in chosen region | Low | High | Pin `us-central1`, verify at Phase 0, keep region in config |
| R12 | Latency makes the conversation feel dead | Medium | High | Measure turn-boundary latency from Phase 1; the acknowledgment-then-inject pattern (§17.5) is the primary defence |
| R13 | Preview model deprecated or changed mid-build | Low | Medium | GA model as default, model IDs in env config |
| R14 | Resume PDF is an image scan and reads as blank | Low | Medium | Gemini handles scans multimodally; validate the digest is non-empty and prompt for re-upload if it is |

---

## 24. Demo Script

Three minutes. Rehearsed. Every beat scripted, including the answers you give the AI.

| Time | Screen | What you do | What you say |
|---|---|---|---|
| 0:00 | Landing | Sign in | "Interview prep is generic. This isn't." |
| 0:10 | Setup | Upload a resume, paste an ML Engineer JD | "It reads my actual resume and the actual job I'm chasing." |
| 0:20 | Digest reveal | Point at the extracted claims and probe angles | "It's already found three claims I'd struggle to defend." |
| 0:30 | Persona cards | Select **Tech Lead** | "And I get to pick who grills me. The Tech Lead is the one who scares me." |
| 0:40 | Live Room | *AI speaks a resume-specific question* | *Say nothing. Let it talk. This is the moment.* |
| 0:50 | Live Room | Answer aloud, deliberately mixing one strong point with one vague claim | *Let the transcript stream visibly.* |
| 1:10 | Live Room | Click **Request hint** | "And when I freeze — it doesn't give me the answer, it asks me a better question." |
| 1:25 | Heatmap reveal | Let the spans illuminate. Hover the amber one. | "Green is proven. Amber is thin. Blue means I asserted something I can't back up. Every colour is traceable to a rubric." |
| 1:45 | Band toast | Point at the band indicator changing | "Two strong answers and it just raised the difficulty. Listen to the next question." |
| 1:55 | Live Room | *AI asks a visibly harder question* | *Let it land, then end the session.* |
| 2:10 | Report | Radar chart | "Strengths and gaps across the domains this specific role demands." |
| 2:25 | Delivery panel | Point at WPM and filler count | "It also graded how I sounded. Fourteen filler words, all of them in the first three seconds of each answer." |
| 2:40 | Roadmap | Scroll the day-by-day plan, click one real link | "And it ends where prep should — eleven days, ordered by prerequisite, with real documentation." |
| 2:55 | Roadmap footer | Click **Retest on day 4** | "Which loops straight back in. Built entirely on Vertex AI." |

**Rules for the rehearsal:**

- **Script your own answers.** Know exactly which sentence will trigger the amber span and which will trigger the blue one. Say them the same way every time.
- Headphones. Always.
- Have the backup video open in a second tab.
- If something breaks, keep talking and move to the next screen. Do not debug live.
- The single strongest moment is 0:40 — the first time the AI speaks a question about *your* project. Give it silence. Don't talk over your own best feature.

---

## 25. Future Scope

Ordered by ratio of value to effort, which is also the order to mention them if a judge asks "what's next?"

1. **Panel mode.** All three personas in one session, handing off. "Now I'll bring in our architect." Highest-value single addition.
2. **Session-over-session progress.** Band trajectory, concept mastery decay, a genuine learning curve. Turns a demo into a product.
3. **Company-specific calibration.** Ingest known interview loop structure per company and shape the plan to match.
4. **Coding round with execution.** Shared editor, sandboxed runner, and the interviewer watching you type.
5. **Spaced repetition over the roadmap.** The retest plan generalised into a schedule.
6. **Multilingual interviews.** The native-audio model covers 24 languages; this is close to free and opens a large market.
7. **Recruiter-facing mode.** Same engine, structured candidate assessment, with a serious and deliberate conversation about bias, auditability, and consent before anyone builds it.

---

## Appendix A — Open Decisions

Things this document has assumed. Revisit if any assumption is wrong.

| # | Assumption | Revisit if |
|---|---|---|
| A1 | Interview Mode is the demo path; Study Mode is compliance coverage | The judging rubric weights the study use case more heavily |
| A2 | English only | Your audience isn't English-first |
| A3 | Single-user sessions, no sharing | A social/comparison feature would score better |
| A4 | 12-minute session cap | Credits are more plentiful than assumed |
| A5 | Firestore over Postgres | You need relational queries for progress analytics |
| A6 | No coding editor | Judges are all backend engineers who'll ask about it |
| A7 | `gemini-live-2.5-flash-native-audio` over the 3.1 preview | The 3.1 preview's voice quality is audibly better in the Phase 0 A/B |

## Appendix B — Prompt Assets Inventory

Every prompt lives in a versioned file under `internal/prompts/`, never inline in Go source. You will iterate on these more than on any code in the project.

| File | Purpose | Model |
|---|---|---|
| `digest.md` | Resume + JD → Session Digest | reasoning |
| `persona_tech_lead.md` | System instruction template | live |
| `persona_architect.md` | System instruction template | live |
| `persona_pm.md` | System instruction template | live |
| `evaluate_turn.md` | Answer → evaluation schema | reasoning |
| `delivery_analysis.md` | Answer audio → delivery schema | reasoning |
| `hint_socratic.md` | Question + partial answer → guiding question | cheap |
| `syllabus_decompose.md` | Topic → dependency-ordered subtopics | reasoning |
| `roadmap_build.md` | Missing concepts → day plan + grounded resources | reasoning + search |
| `injection_state.md` | Coach state update template | live |

## Appendix C — Pre-Demo Checklist

- [ ] `min-instances=1` confirmed on the deployed revision
- [ ] Budget alerts firing correctly
- [ ] Excellent-answer test produces zero red spans
- [ ] Fabricated-claim test produces a blue span
- [ ] Disfluent-answer test produces a non-zero filler count
- [ ] Band change fires within three turns
- [ ] Every roadmap link in the rehearsal resolves
- [ ] Backup video recorded and open in a second tab
- [ ] Headphones packed
- [ ] Vertex usage dashboard confirms all inference is billing to Vertex
- [ ] Demo rehearsed five times, same answers, same timings
