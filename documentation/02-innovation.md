# 02 · Innovation and creativity

> Evaluation criterion 2 — *Innovation & Creativity* (15 marks)
> Originality · creative approach · novel features · differentiation

---

## The one-line differentiator

Every other interview-prep tool grades you **after** the session. Crucible grades
you **during** it, and feeds the grade back into the conversation before the next
question is asked.

That single loop is why the difficulty is *audible* rather than a number on a
dashboard afterwards — and it is the thing that could not be built without the
systems work in this repository.

---

## Eight architecture decisions

Recorded ADR-style during the build. Four are engineering hygiene; four are
genuinely novel and are marked ★.

### ★ AD-2 · Manual turn boundaries instead of voice-activity detection

**What everyone does.** Server-side VAD decides when you stopped talking.

**What we do.** VAD is switched off (`AutomaticActivityDetection.Disabled = true`).
The client sends explicit `activity_start` / `activity_end`; the model does not
begin its turn until told.

**What it buys — four things at once:**

1. **Deterministic boundaries.** A noisy demo hall cannot fire a false end-of-turn.
2. **Echo becomes structurally impossible.** The model cannot mistake its own voice leaking through speakers for user speech, because we control when its turn starts. This retires an entire risk class rather than mitigating it.
3. **Cost.** With manual control the client can gate on energy and simply *not transmit silence*. Live audio is the dominant cost in the system and a ten-minute session is mostly silence.
4. **Injection timing.** We decide the exact moment the model may speak — which is what makes AD-3 reliable.

**The trade-off, stated honestly.** We lose natural barge-in: the persona can no
longer interrupt a rambling candidate. `LIVE_ACTIVITY_MODE=manual|auto` keeps both
available.

### ★ AD-3 · Deadline-bounded injection

The grade for turn *N* must reach the interviewer before turn *N+1* is asked. But
grading takes seconds, and an interviewer sitting silent on stage is fatal.

So the boundary starts a **race**: the real grader against a deterministic
fallback built from data we already hold (current band, coverage sets, the next
unaddressed plan area). Whichever arrives first is injected. The conversation
never waits on a model call; a hang becomes a slightly duller question instead of
silence.

**This is also where the build's most instructive bug lived.** The deadline was
set to 3.5 s from the design doc. Measured grading latency is 5–8 s — so the
fallback won *every single race* and the grader's sharp follow-up never reached
the conversation. The feature was inert and nothing was failing. It was caught by
noticing the injected question length was a constant instead of varying.

The reframe that fixed it: **the deadline is not a silence budget.** The
interviewer is already acknowledging the answer during that window. Raised to 9 s;
injected probe lengths now vary per turn (117/125/134/142/156 chars), which is the
signature of a real grader rather than a canned fallback.

### ★ AD-4 · Server-side confidence gating

The most damaging thing this product can do is tell a candidate their correct
answer was wrong. A prompt instruction is not a defence against that.

So the evaluation schema requires a `confidence` per span, and Go applies a
deterministic rule before anything is persisted:

```
verdict == "incorrect" && confidence < 0.75  →  rewrite to "unsupported"
```

One `if` statement, tunable by env var at 2 a.m. without a redeploy. Result:
**zero false reds across every calibration test**, including an answer composed
entirely of unbacked numbers — which came back `unsupported`, exactly as the
four-verdict taxonomy intends.

**The four verdicts themselves are part of the design.** Most graders are binary.
Separating *unsupported* (you claimed it, you didn't evidence it) from *incorrect*
(that is false) is what makes the feedback fair enough to trust.

### ★ AD-7 · Ghost Session — replay over the identical protocol

A recorded session is served back over the **same WebSocket protocol**, frame for
frame, with zero Vertex calls. The frontend cannot tell the difference and needs
no replay-specific code.

Verified on the deployed service: a full 27-second interview — audio, both
transcripts, frame timings — for **0 tokens and 0 API calls**.

Its purpose is a demo that cannot be killed by venue wifi, a rate limit, or a
Vertex outage. The usual mitigation is "record a backup video", which is visibly
an admission of failure. This exercises the real UI instead. It also turned out to
be the right way to load-test the relay for free.

---

### The four supporting decisions

| | Decision | Why |
|---|---|---|
| **AD-1** | One Go binary on Cloud Run — REST, relay and workers in one process | At ten concurrent sessions, a channel and N goroutines *are* the queue infrastructure. Pub/Sub would buy nothing and cost a class of deployment bug. |
| **AD-5** | Firestore is the source of truth; memory is a cache | Cloud Run session affinity is best-effort. With Firestore authoritative, reconnect, instance-restart survival and idempotent finalization come free instead of being three separate hacks. |
| **AD-6** | Transport-agnostic turn engine | Voice, typed answers and Study Mode are three uses of one code path rather than three implementations. Worth roughly a full phase of build time. |
| **AD-8** | Prompts embedded and content-hashed | The 8-char hash is logged with every call and stamped on the evaluation, so a bad grade can be traced to the exact prompt version that produced it. |

---

## Novel features a judge can see

**Span-level heatmap anchored to your literal words.** The evaluator returns
verbatim excerpts; a four-tier resolver (exact → normalised → fuzzy → drop)
locates them in Go. It **drops** what it cannot place rather than guessing,
because a missing highlight is invisible while a misplaced one attaches a verdict
to words that never made the claim. Measured drop rate: **0%**.

**Three interviewers who are genuinely different.** Distinct rubric weights, probe
doctrine, and distinct Live voices. Same résumé, same JD:

| Persona | Voice | Opening question |
|---|---|---|
| Tech Lead | Charon | "…how did you structure the worker pool to **safely handle billing retries without duplicate charges**?" |
| Architect | Orus | "…how was the worker pool **structured to reliably process** monolithic billing jobs?" |
| PM | Aoede | "**Hi there, welcome!** …how was it **designed to reduce the billing job runtime**?" |

The Tech Lead goes at the failure mode, the Architect at structure, the PM opens
warmly and asks about outcome — exactly what the rubric weights predict.

**Delivery feedback from the audio, not the transcript.** Pace, filler count and
hesitation are analysed from the answer audio. Hesitation is prosodic — pauses
mid-sentence, restarts, trailing off — and no transcript carries it at any
quality. A rule enforced by test: report **counts and observations, never
character judgements**. "You said 'um' 14 times in 90 seconds" is actionable;
"you sound unconfident" is not, and is where a coaching tool becomes cruel.

**A roadmap whose links are verified, not just grounded.** Grounding is not the
guarantee it appears to be — one grounded call returned `groundingChunks: 0`,
meaning the model answered from memory while the tool was enabled. So every URL is
fetched over HTTP server-side and dead ones are dropped. **7 of 7 verified HTTP
200.** Ordering is by *prerequisite*, not by score: buffers before backpressure
before throttling before observability.

**Teach-back as the mastery bar.** In Study Mode, `solid` is reachable only
through a teach-back scoring ≥ 7.5. A subtopic can ace recall, application and
edge-case and still not be solid — because reciting is not understanding, and a
mastery map that cannot tell those apart is decoration.

Next: [03-architecture.md](03-architecture.md).
