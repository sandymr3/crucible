Write ONE drill question.

## Topic

{{TOPIC}}

## Subtopic being tested

{{SUBTOPIC}}

{{SUBTOPIC_WHY}}

## Question type: {{ARCHETYPE}}

{{ARCHETYPE_BRIEF}}

## Already solid — you may build on these, but do not re-test them

{{PROVEN}}

## Rules

- **One question.** Never stack two.
- Under 45 words. This is a drill, not an exam paper.
- Answerable in two to four sentences by someone who understands it, and not
  answerable at all by someone who does not. A question that can be bluffed
  teaches you nothing about them.
- Be concrete. Prefer specific numbers, named scenarios, and real mechanisms
  over "discuss the implications of".
- Do not reveal the answer inside the question.
- No preamble and no scaffolding. Return the question text alone.

## The archetypes

**recall** — can they state it precisely?
> "State the scaled dot-product attention formula and name what each term does."

**application** — can they use it on something concrete?
> "Given a 512-dimension embedding and 8 heads, what is the per-head dimension,
> and what would break if you used 7 heads instead?"

**edge_case** — do they know where it fails?
> "What happens to the softmax if you remove the 1/sqrt(d_k) scaling and d_k is
> large?"

**teach_back** — can they explain it to someone else?
>
> This is the highest-signal question in the product. Name a specific audience
> with specific prior knowledge, because "explain simply" invites vagueness
> while "explain to someone who knows X but not Y" forces real translation.
>
> "Explain KV caching to someone who understands matrix multiplication but has
> never heard of a transformer. You may not use the word 'attention'."
>
> Constraining a word they lean on is what separates understanding from
> recitation.
