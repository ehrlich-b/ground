# Ground — Design Document (v2)

**Ground is a source-anchored encyclopedia of weighted facts.**

Every claim in the system traces back to verbatim quotes from cited sources. Sources carry credibility scores. The system computes a baseline credibility for every source and a baseline groundedness for every claim — and then exposes those credibilities as **free parameters** so anyone can re-render the entire knowledge graph under their own priors.

The interesting part isn't agreement. It's how groundedness moves under different views of which sources to trust.

> **What changed from v1.** The v1 design ("12 LLM personalities argue, EigenTrust extracts truth") didn't work, because identical models with personality prompts converge to the same prior. The seed protocol had to forge contests via per-personality contest quotas — admitting the signal was synthetic. v2 inverts the source of signal: agents extract verbatim quotes from real sources, the audit step is mechanically checkable, and credibilities are user-tunable lenses on top of computed baselines. See [§Migration from v1](#migration-from-v1).

---

## Core principle

> **No claim without a citation. No citation without a verbatim quote. No quote that fails mechanical containment check survives audit.**

Everything in the graph traces back to cached source bodies. The first guard against LLM hallucination is not LLM judgment — it's `strings.Contains(source.body, citation.verbatim_quote)`. If the quote isn't literally in the cached source, the citation is rejected before any further processing.

This is the wall. Once you have it, everything downstream becomes meaningful: agents can be graded by the audit pass rate of their citations; sources can be graded by how often citations *of* them check out; claims can be graded by the credibility-weighted balance of their supporting and contradicting citations.

## What "ground" means now

Three uses of the word, each load-bearing:

1. **Grounded in source.** Every claim has a path to text someone wrote, time-stamped, hash-pinned, archived.
2. **Ground truth as audit signal.** Whether a verbatim quote appears in a cached body is a fact about the bytes, not an opinion. Whether the quote semantically supports the claim is an LLM judgment, but it's a *judgment about a fact*, not a freestyle opinion.
3. **Ground as your own priors.** The credibility you assign to sources is yours. Lenses make those priors movable, comparable, and shareable.

## Data model

```
Source
  id            string
  url           string
  content_hash  string        -- sha256 of fetched body
  body_blob_id  string        -- handle into blob storage
  fetched_at    timestamp
  type          enum(paper, preprint, dataset, govt, news, encyclopedia, blog, book, primary, other)
  metadata      json          -- DOI, author(s), publication, year, etc.

Source_Anchor                  -- admin-set credibility prior
  source_id     string
  credibility   float [0..1]
  set_by        agent_id (admin)
  reasoning     text
  set_at        timestamp

Source_Tag                     -- many-to-many; either auto-derived or admin-set
  source_id     string
  tag           string         -- e.g. "peer-reviewed", "preprint", "ap-news", "industry-funded"

Source_Credibility             -- per-epoch computed baseline
  source_id     string
  epoch_id      int
  value         float [0..1]
  components    json            -- breakdown: anchor=0.95, graph=0.78, audit=0.81

Claim
  id            string
  proposition   text
  status        enum(active, emerging, grounded, refuted, contested, adjudicated)
  groundedness          float [0..1]  -- intrinsic, from citations
  effective_groundedness float [0..1] -- after dep-DAG flow
  contestation  float [0..1]
  embedding     blob

Citation                       -- the heart of the system
  id            string
  claim_id      string
  source_id     string
  verbatim_quote text           -- must literally appear in source body
  locator       json            -- offset/page/section, robust if re-fetched
  polarity      enum(supports, contradicts, qualifies)
  extractor_id  agent_id
  created_at    timestamp

Audit                          -- another agent verifies a citation
  id            string
  citation_id   string
  auditor_id    agent_id
  mechanical    enum(pass, fail)        -- substring check before LLM
  semantic      enum(confirm, misquote, out_of_context, weak, broken_link, null) -- null if mechanical=fail
  verdict       enum(uphold, reject)
  reasoning     text
  created_at    timestamp

Agent
  id            string
  name          string
  role          enum(extractor, auditor, both, observer, admin)
  reliability   float [0..1]   -- audit pass rate, EigenTrust-weighted
  productivity  float          -- extraction throughput, capped contributor

Dependency                     -- claim DAG, unchanged from v1
  claim_id      string
  depends_on_id string
  strength      float [0..1]
  reasoning     text

Lens                           -- first-class user-saveable view
  id            string
  slug          string         -- shareable URL component
  owner_id      agent_id
  parent_lens_id string?       -- forked-from
  description   text
  created_at    timestamp

Lens_Override                  -- per-source absolute or multiplicative override
  lens_id       string
  source_id     string
  mode          enum(absolute, multiplier, exclude)
  value         float

Lens_Tag_Override              -- per-tag multiplier
  lens_id       string
  tag           string
  multiplier    float

Topic, Epoch, Assertion_History -- as v1
```

## Three layers of source credibility

```
┌────────────────────────┐
│   Anchor priors        │  admin-curated, ~100 sources, version-controlled
│                        │  the political center of the system
└────────────────────────┘
            │
            ▼
┌────────────────────────┐
│   Computed baseline    │  per-epoch: anchors + citation-graph + audit aggregate
│                        │  what the algorithm thinks
└────────────────────────┘
            │
            ▼
┌────────────────────────┐
│   Lens overrides       │  per-user: sparse map of {source_id|tag → override}
│                        │  what *you* think
└────────────────────────┘
```

Effective credibility under lens `L`:

```
def credibility(source, lens):
    if lens.has_override(source.id):
        return lens.override(source.id)
    base = computed_baseline[source.id]
    for tag in source.tags:
        if lens.has_tag_override(tag):
            base *= lens.tag_multiplier(tag)
    return clamp(base, 0, 1)
```

Per-source overrides win over per-tag overrides. Lenses can compose (apply parent then child).

## Algorithms

There are two computations: the **epoch** (heavy, periodic) and the **lens render** (cheap, per-request).

### Epoch computation

Run on a schedule (or after a batch of new audits). Persists `source_credibility[epoch_id]`, `agent_reliability[epoch_id]`, `claim_groundedness[epoch_id]`.

#### 1. Agent reliability

Agent reliability is the fraction of their citations that survive audit, weighted by auditor reliability. EigenTrust-style fixed point:

```
reliability(a) = damping * uphold_rate(a)
               + (1 - damping) * prior

uphold_rate(a) = sum over a's audited citations of:
                   (sum over auditors of: reliability(auditor) * uphold_indicator)
                 / (sum over auditors of: reliability(auditor))
                 / count(a's audited citations)
```

Mechanical-fail audits count as a hard reject regardless of auditor (the substring check is objective). Citations with no audits yet contribute nothing — agents earn reliability only when their work has been checked.

#### 2. Source credibility

Three signals combine:

```
credibility(s) = w_anchor * anchor(s)
               + w_graph  * graph(s)
               + w_audit  * audit(s)
```

with `anchor(s)` falling back to a neutral prior (e.g. 0.5) for non-anchored sources. Weights are config; default `(1.0, 0.6, 0.8)` and renormalized only over the components present (an anchored source skips the prior).

- **`anchor(s)`** — direct admin set, in `[0..1]`.
- **`graph(s)`** — EigenTrust over the citation graph: source `s` cites source `t` (extracted from `s`'s reference list when available); credible-citing-credible reinforces. Anchors seed the iteration; non-anchored sources start at the prior.
- **`audit(s)`** — average uphold rate of citations *of* `s`, weighted by extractor and auditor reliability. Sources whose quotes consistently survive audit get credit for being faithfully usable.

Citation graph edges are auto-extracted from source metadata where possible (DOI references, hyperlinks in HTML, bibliographies in PDFs). Missing edges are fine — `graph(s)` falls back to the prior.

#### 3. Claim intrinsic groundedness

This is the linear-in-credibility step that makes lens rendering cheap.

```
g(claim) = clamp(
    (sum_supporting - alpha * sum_contradicting + beta * sum_qualifying)
    /
    (sum_supporting + alpha * sum_contradicting + |sum_qualifying| + epsilon),
    0, 1)

where for each citation c on this claim:
    w_c = credibility(c.source) * reliability(c.extractor) * audit_factor(c)

    sum_supporting    accumulates w_c where c.polarity = supports
    sum_contradicting accumulates w_c where c.polarity = contradicts
    sum_qualifying    accumulates w_c where c.polarity = qualifies (signed by audit consensus)

    audit_factor(c)   ∈ [0..1] = fraction of upholding audits weighted by auditor reliability;
                      0 if any mechanical-fail audit exists; 1 if unaudited (provisional)
```

`alpha` defaults to 1.0 (contradicting evidence weighed equally), `beta` defaults to 0.3 (qualifying evidence partial). Both are config knobs.

#### 4. Effective groundedness via dependency DAG

Identical to v1 — topological pass:

```
eff(c) = g(c) * product over deps d of: eff(d)^d.strength
```

Adjudicated claims pin both `g` and `eff` to their adjudicated value and are skipped in iteration.

#### 5. Status assignment and contestation

Same thresholds as v1: `grounded`, `refuted`, `contested`, `emerging`, `active`, `adjudicated`. Contestation is the variance of credibility-weighted polarities across a claim's citations — high variance = real disagreement in the literature, not in the agent layer.

### Lens render (per-request)

Given a baseline epoch result and a lens `L`, recompute affected claims:

```
1. Build effective credibility map from baseline + L's overrides (sparse merge).
2. For each claim, recompute intrinsic g using the linear formula (step 3 above).
3. Topo-pass over the dep DAG to recompute effective groundedness (step 4).
4. Recompute status under thresholds.
```

Time complexity for a full re-render: `O(|citations| + |dep edges|)`. For graphs <100k claims this is sub-100ms in memory. Cache per `(lens_id, epoch_id)`.

For single-claim views: only the claim's citations and the cone of claims that depend (transitively) on it need recomputation.

### Gradients (free with the linear form)

Because intrinsic groundedness is linear in source credibility, the partial derivative of `g(c)` with respect to `credibility(s)` is computable directly. For any claim, surface the top-N sources by `|∂g/∂credibility(s)|` — "if you stopped trusting this source, the claim's groundedness would move by Δg." This gives users a click-through map of what's load-bearing under any view.

## Agent roles

Agents are research workers, not voters.

### Extractor

Given a topic or claim, finds candidate sources, reads them, proposes citations:

```
POST /api/citations
{
  claim_id: ...,
  source_id: ...,           -- or {url: ..., trigger fetch} to auto-create
  verbatim_quote: "...",    -- must appear in source body
  locator: {page: 4, char_offset: 1820, ...},
  polarity: "supports" | "contradicts" | "qualifies",
  reasoning: "..."          -- why this quote backs that polarity
}
```

The server:
1. Mechanical check: `verbatim_quote` must `strings.Contains` the source body. If not → 400.
2. Persist citation with `audit_factor` provisional (1.0 until audited).
3. Trigger an async audit job.

### Auditor

Given a citation, verifies it. Two-stage:

1. **Mechanical** (server-automatic, idempotent): re-run substring check against current source body. Flags drift if the source was re-fetched and the quote no longer appears.
2. **Semantic** (LLM): re-read the quote in source context. Verdict: `confirm`, `misquote`, `out_of_context`, `weak`, `broken_link`. Auditor writes reasoning. Verdict + reasoning is the unit of work that grades both the auditor and the extractor.

Auditors cannot audit their own citations. Three independent audits per citation gives you a reliable consensus signal.

### Search

Given a topic and a set of existing claims/citations, propose new candidate sources to fetch. Output is `(url, why_relevant)` pairs; the system fetches and queues for extraction. Search is the cheapest agent role and a good way for new agents to build reputation before extraction/audit work.

### Observer

Read-only. Useful for humans browsing without contributing.

### Personality prompts

The 12 v1 personality prompts (empiricist, formalist, etc.) are *not* deleted but are **demoted to search/extraction strategies**. An "empiricist" agent prefers RCTs and meta-analyses when searching; a "historian" agent prefers primary documents. Their *outputs* are citations, not opinions — so the personality variation produces a richer source pool, not synthetic disagreement. The contest quotas and forced-stance hacks are removed entirely.

## Lenses as first-class artifacts

A lens is a saveable, forkable, shareable view. Every page in the UI accepts a `?lens=slug` parameter:

- `ground.ehrlich.dev/claim/c123` — baseline view
- `ground.ehrlich.dev/claim/c123?lens=no-corporate-press` — same claim under a user's lens
- `ground.ehrlich.dev/view/no-corporate-press/leaderboard` — leaderboard under a lens (sources only — agent reliability is *not* lens-dependent)

### Composition

Lens `B` can declare `parent_lens_id = A`. Effective overrides are A's overrides ∪ B's overrides, with B winning conflicts. This lets users build hierarchies:

```
strict-academic
  ├── medicine-only-rcts
  └── physics-prefer-primary
```

### Diff view

Whenever a non-default lens is active, the UI shows a diff banner:

> "Your lens `no-corporate-press` differs from baseline on 23 sources. Top affected claims: [c14] (0.81 → 0.42), [c19] (0.77 → 0.31), ..."

This is the antidote to confirmation-bias-with-sliders. Every lens view shows what it's costing you against the baseline.

### What lenses cannot touch

The audit record is factual; lenses cannot change it. Specifically:

- **Agent reliability** is computed off baseline only. Audit pass rate is objective; if a lens could move it, agents would game lenses.
- **Citation existence and verbatim quote** is immutable record.
- **Anchor priors** are admin-curated baseline; lenses sit above them, never edit them.

## Bootstrapping order

A cold-start protocol that produces a meaningful graph without any LLM-vs-LLM theater:

1. **Anchor curation.** Hand-tag ~100 sources with credibility priors. Tier 1 (≥0.9): peer-reviewed top journals, primary-source government datasets, named encyclopedias for definitional claims. Tier 2 (0.7–0.85): solid secondary sources. Tier 3 (0.4–0.6): mainstream news. Tier 4 (≤0.3): blogs, advocacy. Anchors are public, version-controlled, and contestable via PR.

2. **Axiom seeding.** [FACTS.md](FACTS.md) — adjudicated trust-anchor claims, each with ≥2 citations to anchored sources, each with verbatim quotes. These bootstrap the dependency DAG with credibility-pinned roots.

3. **Source ingestion.** Fetch and cache the first ~1000 sources covering the topic taxonomy. arXiv, PubMed Central, OpenAlex, government open data, Wikipedia (as secondary tier 2).

4. **Extraction round.** Agents pick topics and propose claims with citations. Citations rejected at the mechanical-check wall don't count. The result is a graph of claims whose intrinsic groundedness is computable from cited sources alone.

5. **Audit round.** Agents audit each other's citations. This is where agent reliability gets its first signal. Each citation gets ≥3 independent audits. Audits with reasoning, not just verdicts.

6. **Dependency mapping.** Agents propose `claim depends_on claim` edges with strength and reasoning. Cycle detection at insertion. Same as v1.

7. **Epoch computation.** Run the per-epoch pipeline. First baseline credibilities, reliabilities, and groundednesses are persisted.

8. **Open the gates.** External agents (humans, other models, other organizations) can now register and contribute. They start at the prior; their reliability emerges from audit results.

## Schema migration from v1

The v1 schema is mostly preserved. Net changes:

**New tables** (additive):
- `sources`, `source_anchors`, `source_tags`, `source_credibility`
- `citations`, `audits`
- `lenses`, `lens_overrides`, `lens_tag_overrides`

**Modified tables**:
- `claims`: same shape, semantics carry over
- `agents`: add `role`, `reliability`, `productivity`; deprecate `accuracy`, `contribution`, `weight` (keep columns NULL during transition, drop in a later migration)
- `assertions`: deprecated. Citations replace assertions. The `assertions` table can be migrated row-by-row to `citations` for any v1 data with `stance=support` and an attached source URL — but most v1 assertions had no verbatim quote and will be dropped on reboot.
- `reviews`: deprecated. Audits replace reviews. Not migrated.

**Migration path**: write `002_v2_schema.sql` that creates the new tables. Write `003_drop_v1.sql` later, after a clean run on v2 confirms. Don't drop on the same migration as the rebuild — leave a recovery window.

## Algorithms — pseudocode summary

```
def epoch():
    fetch_pending_sources()
    run_mechanical_checks_on_new_citations()
    iterate_source_credibility_graph()    # EigenTrust over citation graph + audit aggregate
    iterate_agent_reliability_graph()     # EigenTrust over audit verdicts
    for claim in claims:
        claim.g    = linear_intrinsic(claim, computed_credibility, agent_reliability)
    topo_pass:
        for claim in topological_order(deps):
            claim.eff = claim.g * product(eff(d)^d.strength for d in deps_of(claim))
    persist epoch result
    persist source_credibility, agent_reliability, claim.g, claim.eff

def render(lens):
    eff_cred = merge(baseline_credibility, lens.overrides, lens.tag_overrides)
    for claim in claims:
        claim.g_lens = linear_intrinsic(claim, eff_cred, agent_reliability)
    topo_pass:
        for claim in topological_order(deps):
            claim.eff_lens = claim.g_lens * product(eff_lens(d)^d.strength for d in deps_of(claim))
    return {claim_id: (g_lens, eff_lens, status_lens) for claim in claims}
```

## Scaling and storage

- **Source bodies** are content-addressed. Store on disk under `~/.ground/blobs/{sha256}` or in S3-compatible storage. Database holds the hash and metadata only.
- **Re-fetching** sources periodically (weekly?) and noting drift is necessary. If the body changes, mechanical check is re-run on all citations of that source. Drift is news; surface it.
- **Citation count** is the dominant scaling factor. At 100k claims with average 4 citations each = 400k citations, ~1.2M audits if 3 per citation. SQLite handles this easily.
- **Lens render cache** keyed by `(lens_id, epoch_id)`. Invalidate on lens update or new epoch. A lens render result is just a sparse delta against baseline — most claims are unchanged.

## Risks and what we do about them

| Risk | Mitigation |
|---|---|
| LLM hallucinates a quote that isn't in the source | Mechanical substring check before any LLM step. Hard gate. |
| LLM hallucinates the *source itself* (URL doesn't exist) | Fetch must succeed and produce non-trivial body before the source is created. Reject 404, paywalls, JS-walls with explicit type tag. |
| Citation laundering (real source, but misrepresents what it says) | Three-way audit with semantic verdict. Misquote/out-of-context verdicts hurt extractor reliability. |
| Anchor list capture (whoever sets anchors controls the system) | Anchors are version-controlled and small (~100). PRs are public. Diff lens against baseline is always one click. Users can ignore anchors entirely via lens. |
| Paywalls and JS-rendered content | Tier the source type. Unverifiable sources get a credibility floor and a "unverifiable" tag. Lens can boost or kill the tag. Start with open-access content; expand as fetcher improves. |
| Adversarial agents flooding low-quality citations | Rate limits per agent. Reliability decays toward prior on audit failures. Productivity is a capped contributor — no exponential influence. |
| Sock-puppet audit collusion | Audit assignments are pseudo-random and routed through reliability-weighted sampling. Collusion requires a critical mass of high-reliability fakes — hard to bootstrap. |
| Confirmation bias via lenses | Diff-against-baseline banner is always visible. Lens forks track provenance. The lens itself is a public artifact. |
| Source goes offline / changes / rotates URL | Content hash + cached body insulates the system. Drift detection on re-fetch. Sources don't disappear from the record even if they vanish from the web. |

## Migration from v1

| v1 concept | v2 status |
|---|---|
| Agents (id, name, scores) | Kept; rescoped: `accuracy`/`contribution`/`weight` → `reliability`/`productivity` |
| Claims (proposition, status, groundedness, deps) | Kept; same schema with citations replacing assertions as the input signal |
| Assertions (support/contest/refine + confidence) | **Removed.** Citations with polarity replace them. |
| Reviews (helpfulness ratings) | **Removed.** Audits with verdicts replace them. |
| Personality prompts | Demoted to search strategies. No more contest quotas. |
| Forced contest mechanic | **Deleted.** Disagreement comes from real source contradictions. |
| Adjudicated claims | Kept, now require citations. |
| Topic taxonomy | Kept unchanged (TOPICS.md). |
| Dependency DAG | Kept unchanged. |
| EigenTrust iteration | Kept; applied to source citation graph and to audit graph instead of agent assertion graph. |
| Truthiness explorer (dep sliders) | Generalized: now you slide source credibilities and tags too. Lenses are the saveable form. |
| 12-agent seed orchestration script | Kept; tasks rewritten for extract/audit/search rounds. |
| D3 graph viz | Kept; nodes now include sources alongside claims and topics. |
| Single binary, Cobra CLI, JWT API | Kept unchanged. |

## What success looks like

A user lands on `ground.ehrlich.dev/claim/c-landauer-experimental`:

> **"Erasing one bit of information dissipates at least kT ln 2 of energy, and this minimum has been experimentally approached."**
>
> **Groundedness**: 0.91 (baseline). Effective: 0.89.
> **Status**: grounded.
>
> **Citations** (4 supporting, 0 contradicting, 1 qualifying):
> - [Berut et al., Nature 2012, p.187]: *"...we measure the heat dissipated during a logically irreversible memory erasure procedure..."* — supports, audited 3/3 uphold
> - [Jun et al., PRL 2014, p.2]: *"...the work performed during a single bit erasure can approach the Landauer limit..."* — supports, audited 3/3 uphold
> - [Wikipedia: Landauer's principle, retrieved 2026-04-15]: *"...kT ln 2 = 2.85 zJ at room temperature..."* — supports, audited 2/3 uphold (one weak)
> - [Sagawa & Ueda, PRL 2009]: *"...this lower bound applies under specific thermodynamic boundary conditions..."* — qualifies, audited 3/3 uphold
>
> **Try a different lens**:
> - `?lens=primary-only` → drops to 0.84 (Wikipedia excluded)
> - `?lens=industry-funded-discount` → unchanged
> - `?lens=skeptic-physics` → 0.71 (qualifies weighted heavier)
>
> **Top sources by gradient**: removing Berut 2012 → Δ -0.27. Removing Jun 2014 → Δ -0.21.

That's the product. A claim that's defensible, sourced, audit-checked, and tunable.

## Out of scope (for now)

- Cross-lingual sources. English first; multilingual is a v3 concern.
- Real-time fetching during request. All source fetches are async; the request returns what's cached.
- Distributed ground instances federating. One canonical instance for now; lenses are the federation primitive at the user level.
- Reasoning chain validation (i.e., not just "this source supports this claim" but "this argument from these claims to that conclusion is valid"). Out of scope; that's a logic system, not an encyclopedia.

## File pointers

- [README.md](README.md) — pitch and quick start
- [TOPICS.md](TOPICS.md) — topic taxonomy (unchanged from v1)
- [FACTS.md](FACTS.md) — adjudicated axioms (now with required citations)
- [SKILLS.md](SKILLS.md) — agent integration guide
- [TODO.md](TODO.md) — implementation roadmap
- [CLAUDE.md](CLAUDE.md) — codebase conventions
