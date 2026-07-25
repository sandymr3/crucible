Decompose a topic into a dependency-ordered syllabus for someone who is about
to be tested on it.

## Topic

{{TOPIC}}

## Depth requested

{{DEPTH}} — {{DEPTH_NOTE}}

## Their own syllabus or scope, if they gave one

{{SYLLABUS_TEXT}}

## What to produce

A list of subtopics with **real prerequisite edges** between them.

The edges are the point. A flat list of subtopics is a table of contents;
a dependency graph is a study order. Get the edges right and the plan teaches
itself in the correct sequence.

For each subtopic:

- **id** — `s1`, `s2`, `s3`. Stable and short.
- **name** — what a learner would call it, not a textbook chapter heading.
  "Why divide by sqrt(d_k)" beats "Normalisation considerations in the scaled
  dot-product formulation".
- **prereqs** — the ids that must be understood **first**. Only direct
  prerequisites: if C needs B and B needs A, C lists only B. Listing A too adds
  a redundant edge and flattens the graph.
- **depth** — 1 for something needing no prior subtopic here, rising from there.
- **why** — one short sentence on what this unlocks. Shown to the learner, so
  make it a reason rather than a restatement.

## Rules

- **No cycles.** A depends on B depending on A makes both permanently
  unreachable, and the drill loop stalls.
- **Every prereq id must exist in this list.** A dangling reference makes that
  subtopic unreachable forever.
- **At least one subtopic must have no prerequisites**, or nothing can start.
- Order the array roughly foundational-first. The consumer sorts by dependency,
  but a sensible order makes the output readable.
- Prefer 6 to 9 subtopics unless the depth note says otherwise. Twelve is the
  hard ceiling — past that it is a reading list nobody finishes.
- Each subtopic must be small enough to genuinely learn in one sitting, and
  specific enough to ask a sharp question about. "Attention" is too big.
  "Why the scaling factor is 1/sqrt(d_k)" is right.

## Worked example of the shape

For "Transformer attention":

```
s1  Dot-product similarity              prereqs: []        depth: 1
s2  Scaled dot-product attention        prereqs: [s1]      depth: 2
s3  Why divide by sqrt(d_k)             prereqs: [s2]      depth: 3
s4  Multi-head attention                prereqs: [s2]      depth: 3
s5  KV caching at inference             prereqs: [s4]      depth: 4
```

Note that s3 and s4 both build on s2 and are independent of each other — the
graph branches. Do not force everything into a single chain.
