# Task: Dependency Mapping

You are mapping logical dependencies between claims in Ground. A dependency `A → B` (claim `A` depends on claim `B`) means: **if B turned out to be false or much weaker, the case for A would be weaker too.**

Dependencies form a DAG. Cycles are rejected by the server. Choose direction carefully.

## Input

A batch of claims, plus the adjudicated axioms they may depend on:

```
CLAIMS:
{{CLAIMS}}

AXIOMS (adjudicated, can only be a target of dependency, never a source):
{{AXIOMS}}
```

## What counts as a dependency

- **Logical**: A is an instance of, a corollary of, or a special case of B.
- **Empirical scaffolding**: A's truth is supported by the same line of evidence that supports B (sometimes — be careful here, prefer a citation over a dependency if the relation is "A's source is also B's source").
- **Conceptual**: A's intelligibility presupposes B (e.g. claims about evolutionary adaptation depend on claims about heritable variation).

## What is NOT a dependency

- **Topical adjacency**: "both about cosmology" is not a dependency.
- **Reverse causation**: "B implies A" is not "A depends on B". Get the direction right.
- **Author overlap**: "same author wrote both papers" is not a dependency.
- **Mutual reinforcement**: if you'd want to draw `A → B` and `B → A`, you're double-counting; pick the one that's actually structural and skip the other.
- **Trivial chains**: don't map "A depends on B" if there's already a chain `A → C → B` doing the same work.

## Strength

A real number in `[0, 1]`:
- `1.0` — A is logically equivalent to or directly entailed by B.
- `0.7–0.9` — A is a clear instance of or conditioned on B.
- `0.4–0.6` — A's case substantially uses B as evidence.
- `0.1–0.3` — A's case marginally relies on B; consider whether this dependency is worth recording.
- Below `0.1` — don't record it.

## Output

JSON array. No prose, no markdown fences.

```json
[
  {
    "claim_id": "id-of-A",
    "depends_on_id": "id-of-B",
    "strength": 0.8,
    "reasoning": "One sentence on the relation."
  }
]
```

- `claim_id` — the claim that depends.
- `depends_on_id` — the claim it depends on (can be an axiom).
- `strength` — see above.
- `reasoning` — one sentence; the auditor reads this.

Self-loops (`claim_id == depends_on_id`) are rejected. Cycles are rejected. Duplicate edges are silently dropped. An empty array is fine.
