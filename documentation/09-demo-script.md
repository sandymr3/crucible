# 09 · Demo script

> Evaluation criterion 5 — *Presentation & Demo* (10 marks)
> Clear PPT · good explanation · well-structured demo · easy to understand

---

## Before you start

| | |
|---|---|
| **Headphones on.** | Manual activity detection makes echo structurally impossible, but speakers still make the room hear both sides at once. |
| **Check `/health` returns 200.** | Ten seconds. Do it while the previous team presents. |
| **Have the Ghost Session command already typed** in a second terminal, not typed under pressure. |
| **Confirm `min-instances=1`** on the deployed revision, so the first interaction has no cold start. |
| **Know your two numbers**: 966 ms turn boundary, 0 false reds. If you only land two facts, land those. |

---

## The five-minute run

### 0:00 — The hook (30 s)

> "Most interview prep is a quiz with a chat window bolted on. You answer, it
> scores you, and you learn nothing about *where* you were weak.
>
> Crucible is a live spoken conversation that gets harder when you're good."

Do not explain the architecture yet. Land the difference first.

### 0:30 — The problem (45 s)

Three failures, one line each — a static bank can't follow up; nothing hears *how*
you answer; a 6/10 tells you that you failed, not where.

> "The gap isn't question generation — models have been good at that for years.
> It's that nothing hears you, adapts mid-conversation, and shows you where you
> were vague. All three need the answer to arrive as speech, in real time, inside
> a session that's still open. That's a systems problem, not a prompting problem."

### 1:15 — The live interview (2 min) — **the centrepiece**

Upload the résumé, pick **Tech Lead**, start.

**Point at the first question as it is spoken.**

> "It just named a project from the résumé. That's not a template — the résumé
> and JD were read into a digest with specific probe angles before we connected."

Give a **deliberately vague** answer about backpressure. Roughly 20 seconds. Then
click Done.

**Now point at the screen and stop talking for a beat.**

> "Those are my own words, graded span by span. Amber is 'true but thin'. Blue is
> 'you claimed it, you didn't evidence it'.
>
> Note what's *not* there: red. An unbacked claim is not the same as a false one,
> and any grader that conflates them loses your trust the first time it's unfair.
> Across every calibration test we've run, we have **zero false reds**."

Then give a **strong** answer on the next question and let the band move.

> "Band 3 to 4. And this is the part I'd point at if you only remember one thing:
> that grade was injected back into the *same open session* before the next
> question was asked. The difficulty didn't change on a dashboard afterwards —
> it changed in the conversation. That's adaptive difficulty you can hear."

### 3:15 — Report and roadmap (45 s)

> "Radar across the role's domains. Delivery metrics — 156 words per minute,
> 13 fillers — computed from the *audio*, because hesitation is prosodic and no
> transcript carries it.
>
> And the roadmap is in prerequisite order, not priority order: buffers before
> backpressure before throttling. Every link here was fetched over HTTP before you
> saw it. Seven out of seven resolve. Grounding alone isn't enough — one grounded
> call came back with zero grounding chunks, meaning the model answered from
> memory while the tool was on."

### 4:00 — The engineering (45 s)

One slide, three sentences:

> "One Go binary on Cloud Run — REST, the WebSocket relay and the grading pool.
> The relay isn't an optimisation; Vertex wants an OAuth2 token from a service
> account, and you cannot put that in a browser, so every audio frame goes through
> us.
>
> 966 milliseconds turn boundary, ten concurrent sessions with zero gaps,
> seventeen chaos checks, 125 tests. All measured on the deployed service."

### 4:45 — Close (15 s)

> "The backend is live right now at that URL. The Live model ships 24 languages —
> regional-language interview prep is the next thing we build, and it's config,
> not a rebuild."

---

## The Ghost Session — your insurance

If venue wifi degrades, Vertex rate-limits, or anything else goes wrong:

```bash
go run ./cmd/wsprobe -session <replay-session-id> -token <id-token>
```

A recorded 27-second interview replays over the **identical WebSocket protocol** —
same frames, same timings, real audio — with **zero Vertex calls**. The UI cannot
tell the difference.

Create one with `POST /v1/sessions {"mode":"replay","fixtureId":"demo-ml-engineer"}`.

**Say so out loud if you switch to it.** It reads as engineering foresight, not as
a save:

> "I'm switching to our replay mode — same protocol, same frames, recorded from a
> real session. We built it because a demo shouldn't depend on venue wifi."

That is a stronger moment than a demo that merely worked.

---

## Answers to the questions you will get

**"Is this actually live or a video?"**
> Live. Here's the health endpoint, and here's the Cloud Run URL — and this
> instance is serving right now.

**"What if it grades someone wrong?"**
> That's the failure mode we designed hardest against. The schema requires a
> confidence per span, and any `incorrect` below 0.75 is rewritten to
> `unsupported` in Go before it's persisted — a prompt instruction isn't a
> defence. Zero false reds across every test, including an answer made entirely
> of unbacked numbers.

**"How much does a session cost?"**
> We can tell you exactly — every Vertex call is metered into a ledger split by
> model and by audio-versus-text tokens. `GET /sessions/{id}/usage`. Live audio
> dominates; that's why the client doesn't transmit silence and why a 90-second
> idle timeout closes the billing connection.

**"What doesn't work?"**
> Three things. WebSocket reconnect isn't built — a dropped socket ends the
> session, and the resumption handles are already being emitted but nothing
> consumes them. Evaluation runs 5 to 8 seconds against our 4-second target,
> though the conversation never waits on it. And our load test proves *our*
> concurrency, not Vertex's, because ten simultaneous live sessions would cost
> real credits.

**"Why Go rather than Python?"**
> Each live session is two goroutines and a channel; the grading pool is a
> buffered channel and N workers. That's the entire queue infrastructure, in the
> standard library. And it ships as one static binary into a distroless
> container.

**"What's the hardest part?"**
> Turn boundaries. We switched off voice-activity detection and made the client
> own the boundary explicitly. That makes it deterministic in a noisy hall, makes
> it structurally impossible for the model to hear itself, and lets us skip
> transmitting silence — which is the dominant cost.

---

## Do not

- **Do not explain the architecture before the demo.** Show it working, then say how.
- **Do not narrate while the AI is speaking.** Let the room hear it. The voice is the product.
- **Do not click Done early.** The turn closes on transcription, and a clipped answer grades badly and makes the grader look wrong.
- **Do not claim the frontend is finished** if it isn't. Say the backend is complete and deployed, and show it through the real protocol.
- **Do not apologise for the gaps.** Name them flatly if asked. A team that knows exactly what's unbuilt reads as more competent than one that claims everything works.
