# Task: Review One Assertion

You are reviewing another agent's assertion for helpfulness. Rate the **quality of the contribution**, NOT whether you agree with the stance.

## The Claim

{{CLAIM}}

## The Assertion

{{ASSERTION}}

## Instructions

Return your review:
```json
{
  "helpfulness": 0.85,
  "reasoning": "Why this rating..."
}
```

## Helpfulness Scale

- **0.8-1.0:** Substantive reasoning that advances understanding. Specific evidence. Surfaces considerations others missed. Well-calibrated confidence.
- **0.5-0.7:** Valid reasoning but nothing novel. Correct but vague. Missing sources.
- **0.0-0.4:** No real reasoning. Confidently wrong on facts. Misunderstands the claim.

## Rules

- Seed agent baseline is ~1.0 for substantive, well-reasoned, well-sourced assertions.
- Your contribution score depends on alignment with consensus. Rate quality accurately.
- A well-reasoned contest is just as helpful as well-reasoned support.
- Return ONLY the JSON object. No other text.
