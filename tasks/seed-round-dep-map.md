# Task: Map Dependencies Between Claims

You are a seed agent in the Ground knowledge base. Your job is to identify epistemic dependencies between claims — where the truth of one claim depends on or is supported by the truth of another.

## Your Assigned Topics

{{TOPICS}}

## Claims to Analyze

{{CLAIMS}}

## Axioms (Potential Dependency Targets)

{{AXIOMS}}

## Instructions

For each claim above, consider whether its truth depends on any other claim in the list or any axiom. A dependency means: "if the depended-on claim were false, this claim's groundedness should decrease."

Return a JSON array of dependency edges:
```json
[
  {
    "claim_id": "id-of-claim-that-depends",
    "depends_on_id": "id-of-claim-it-depends-on",
    "strength": 0.7,
    "reasoning": "Why this dependency exists..."
  }
]
```

If no dependencies exist between these claims, return an empty array: `[]`

## Dependency Types to Look For

- **Logical entailment**: claim A logically requires claim B to be true
- **Evidential support**: the evidence for claim A is largely the same evidence for claim B
- **Mechanism sharing**: claim A describes a phenomenon whose mechanism is described by claim B
- **Definitional dependence**: claim A uses a concept whose validity depends on claim B
- **Cross-domain bridge**: claim A in one field depends on a finding (claim B) in another

## Strength Guide

- **0.8-1.0**: Strong logical dependency. If B is false, A is almost certainly wrong.
- **0.5-0.7**: Moderate dependency. B provides significant evidential support for A.
- **0.2-0.4**: Weak but real dependency. B is relevant context that affects A's plausibility.

## Rules

- Only identify genuine dependencies. Don't connect claims just because they're on the same topic.
- A claim can depend on multiple other claims. A claim can be depended on by many claims.
- Claims can depend on axioms. Axioms cannot depend on claims (they are fixed).
- Do not create self-dependencies.
- Strength must be between 0 and 1.
- Return ONLY the JSON array. No other text.
