# 07 · Scalability and real-world impact

> Evaluation criterion 6 — *Scalability & Real-World Impact* (10 marks)
> Real-world applicability · scalability · business potential · long-term impact

---

## How it scales

### Instances are disposable, and that is a design decision

**Firestore is the source of truth; the in-memory session is a cache** (AD-5).
Every piece of state that matters — turns, transcripts, coverage sets, band, band
history, job status — is written the moment it changes.

Cloud Run session affinity is best-effort, not a guarantee, so a reconnect can
land on a different instance. Because Firestore is authoritative, three things
come free that would otherwise be three separate hacks: reconnect, survival of an
instance restart, and idempotent finalization. The cost is roughly two extra
Firestore writes per turn, which is irrelevant.

The consequence for scaling: **any instance can serve any request.** Horizontal
scale is a `--max-instances` change.

### Current deployment envelope

| Setting | Value | Why |
|---|---|---|
| `--min-instances` | 1 | Removes cold start from the first demo interaction |
| `--max-instances` | 5 | Deliberately capped for a credit-limited hackathon project, not a technical ceiling |
| `--concurrency` | 20 | Sessions per instance |
| `--cpu` / `--memory` | 2 / 2 GiB | |
| `--no-cpu-throttling` | on | **Required.** A throttled instance stops relaying audio between requests. |
| `--timeout` | 3600 s | Long-lived WebSockets |
| `--session-affinity` | on | Best-effort; correctness does not depend on it |

Nominal headroom at these settings is 5 × 20 = 100 concurrent sessions. **We have
measured 10** (see below), so 100 is arithmetic, not evidence.

### Measured concurrency

```
session 1..10   audio 18183 ms   gaps 0
streaming audio : 10 of 10      server errors : 0
with gaps       : 0             dropped clients : 0
```

Ten concurrent sessions, byte-identical audio, zero sequence gaps, zero errors,
zero clients dropped for a full outbound buffer.

**The honest caveat, stated in full:** this test uses replay sessions. It
exercises the whole relay path — upgrade, auth, ownership checks, guardrails, the
write pump, audio framing, sequence numbering — but it does **not** open ten
simultaneous Vertex connections. Concurrency of our service is proven; concurrency
of the model path is not, and given the 429s already seen during the build it
would likely rate-limit. That is the next test to run, and it costs credits.

### Where it would break first, and what the fix is

| Bottleneck | Symptom | Fix |
|---|---|---|
| Vertex Live quota | 429s under concurrent load | Quota increase; the retry path already backs off correctly with decorrelated jitter |
| Worker pool saturation | Grading falls behind the conversation | The pool is a buffered channel and N goroutines — one config change. Injection already deadline-bounds, so the conversation degrades gracefully rather than stalling. |
| Workers die with the instance | Unfinished grades | Every job is idempotent and re-drivable from Firestore state (AD-1/AD-5). Beyond ~50 concurrent sessions this is where Cloud Tasks would earn its complexity. |
| Firestore hot document | Contention on the daily counter | Already transactional; shard the counter if it ever matters |

---

## Unit economics

Every Vertex call is metered into a Firestore ledger split by model and by
audio-versus-text tokens, and accumulated onto the session. A real per-session
record:

```
total=378  audio_in=127  audio_out=163  calls=1
```

`GET /v1/sessions/{id}/usage` returns this breakdown per session. **"Here is our
actual per-session cost" is a number we can quote rather than a guess** — which is
the difference between a viable product and a demo.

### Cost is designed down, not hoped down

Live audio is the dominant cost and everything else is rounding error, so the
architecture attacks it directly:

- **Manual activity detection lets the client not transmit silence at all** (AD-2). A ten-minute session is mostly silence.
- **A 90-second idle timeout closes the billing connection**, so a forgotten tab stops costing money.
- **Answers under 15 words are never sent for grading.**
- **Delivery analysis is queued only when audio exists**, so typed answers cost nothing extra.
- **Digest bounds** (max 8 claims, 6 plan areas) keep the live system instruction small — the Live API charges for it on every turn.
- **Nine guardrails** cap duration, daily use and concurrency.

---

## Real-world applicability

### Who needs this now

**Campus placement cells** are the obvious wedge and a large one in India.
Hundreds of final-year students need mock interviews; a handful of staff cannot
supply them. The scarce resource is *a senior engineer's hour*, and this is the
only part of the pipeline that can be automated without becoming a quiz.

**Bootcamps and upskilling platforms** already sell outcomes and have no way to
rehearse the spoken component.

**Individual career switchers** are the hardest-served group: generic question
banks are useless to someone whose résumé does not yet match the role, and this
system probes what they *have* done and maps it onto what the JD *wants*.

### Why it stays useful after the novelty

Two artifacts survive the session: a report that says **where** you were weak, and
a roadmap ordered by prerequisite with links that were verified before you saw
them. `POST /retest` then materialises a follow-up session that inherits the
digest, JD and résumé and starts **one band above** where you finished — the point
being to prove the gap closed, not to repeat the same difficulty.

That closes the loop into a habit rather than a one-off.

---

## Business potential

| Model | Fit |
|---|---|
| **B2B2C per-seat** — placement cells, bootcamps | Best fit. Cohort dashboards fall out of the existing report aggregation, budgets already exist, and it is priced against staff time rather than against consumer willingness to pay. |
| **B2C freemium** | The daily cap (5 sessions) is already the free tier's shape. Voice minutes are the natural metered unit. |
| **API / white-label** | The evaluation and roadmap paths are already REST and transport-agnostic (AD-6); an LMS could consume them without the voice layer. |

The guardrails are not just a hackathon safety measure — **metered voice minutes
with a hard per-user cap is the pricing model**, already implemented.

## Long-term impact

**The multilingual opening is the largest one, and it is close.** The Live model
ships **24 languages and 30 HD voices**. Interview preparation in Indian regional
languages is almost entirely unserved, and the people it would serve are exactly
those least likely to have a network of seniors to practise with. Reaching it is
config plus prompt work, not a rebuild — the persona system already carries voice
per persona.

**Study Mode generalises the whole machine past interviews.** It reuses the
evaluator, span anchoring, coverage sets and roadmap *unchanged*; only ingestion
and question generation are new. Decomposing "Transformer attention" produced
eight subtopics with genuine branching prerequisites, and mastery requires a
**teach-back**, not recall. That is a general-purpose tutor sitting inside the
same codebase.

**Accessibility.** Every voice path has a typed equivalent on the identical
evaluation path (AD-6), so the product works for someone who cannot or would
rather not speak — without a second implementation to maintain.

**The delivery-feedback rule matters at scale.** Output reports *counts and
observations, never character judgements* — "you said 'um' 14 times in 90 seconds,
mostly right after each question", never "you sound unconfident". A test asserts
the output never contains "unconfident", "nervous" or "you seem". This is where a
coaching tool most easily becomes cruel, and a character judgement is something
the person cannot act on.

Next: [08-setup-and-deployment.md](08-setup-and-deployment.md).
