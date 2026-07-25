Build a day-by-day study plan for a candidate preparing for a {{ROLE_TITLE}}
interview. They have **{{HORIZON_DAYS}} days**.

## What they need to close, most important first

{{CONCEPT_LIST}}

## What they already proved — do NOT put these in the plan

{{PROVEN_LIST}}

## Structure

One concept per day. Fewer days with real depth beats filling every slot.

The concepts above are listed by priority, but **order the plan by what has to
be understood first**. Learning is ordered even when priorities are not: there
is no point studying a distributed consensus optimisation on day one if the
underlying replication model lands on day four.

For each day:

- **focus_concept** — one concept from the list above. Do not invent new ones.
- **why_this_matters** — one sentence connecting it to their interview
  performance and to the role. Written to them, and honest without being harsh.
- **estimated_minutes** — realistic. 45 to 90 for most concepts.
- **resources** — one to three links. See the rules below.
- **practice_task** — something to **do**, not read. Reference their own
  projects where you can. "Take your ingestion pipeline and write down, in four
  sentences, what happens to the producer when the consumer stalls for thirty
  seconds. If you can't, that's the gap."
- **self_check** — a question they can answer to know whether it landed.
  "Explain the difference between a bounded buffer and backpressure without
  using the word 'buffer' twice."

## Resource rules — read carefully

**Every URL you return is fetched over HTTP and verified before the candidate
sees it. Invented URLs are discarded silently, and that day is left with no
resources at all.** So a plausible-looking guess is strictly worse than
returning fewer links.

Search for real pages. Prefer, in this order:

1. Official project documentation (`kafka.apache.org`, `docs.python.org`,
   `pytorch.org`, `cloud.google.com`, `postgresql.org`, and equivalents)
2. Specifications and RFCs
3. `arxiv.org` papers, for anything research-shaped
4. University course pages (`.edu`, `.ac.uk`)
5. Engineering blogs of the companies that built the system in question

Do **not** use: {{EXCLUDED_SITES}}, or any content farm, answer-scraping site,
or SEO listicle.

Prefer a deep link to the specific page over a site's front page. "The Kafka
documentation" is not a resource; the consumer-lag monitoring section of it is.

## summary

Two or three sentences addressed to the candidate. Name what is solid before
what is not. This is the first thing they read after being told what they got
wrong, so it should be accurate rather than either flattering or brutal.
