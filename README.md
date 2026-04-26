# Ground

A source-anchored encyclopedia of weighted facts.

Every claim in Ground traces back to verbatim quotes from cited sources. Sources carry credibility scores. The credibilities are exposed as **free parameters** — you can adjust how much you trust any source (or whole tag, like "all news wires" or "all preprints") and re-render the entire knowledge graph in milliseconds.

The interesting part isn't agreement. It's how groundedness moves under different views of which sources to trust.

## Core principle

> No claim without a citation. No citation without a verbatim quote. No quote that fails mechanical containment check survives audit.

Every citation must contain a literal substring of the cached source body. The first guard against LLM hallucination is `strings.Contains(source.body, citation.verbatim_quote)` — not a model judgment. Citations that fail this check are rejected before any further processing.

Once you have that wall, everything downstream becomes meaningful: agents are graded by audit pass rate; sources by how often citations of them check out; claims by the credibility-weighted balance of their supporting and contradicting citations.

## How it works

**Agents are research workers, not voters.** They do three things: search (find sources for a topic), extract (propose citations with verbatim quotes), audit (verify other agents' citations). Agent reliability is the weighted fraction of their citations that survive audit — an objective, falsifiable signal.

**Sources are graded in three layers**:

1. **Anchor priors** — admin-curated, ~100 sources, public, version-controlled
2. **Computed baseline** — per-epoch from anchors + citation graph + audit aggregate
3. **Lenses** — your overrides on top, sparse, saveable, shareable

**Claims have intrinsic groundedness** computed as a credibility-weighted balance of supporting and contradicting citations. Effective groundedness then flows through a dependency DAG (a claim resting on shaky foundations is weaker than it looks regardless of its own evidence).

## Lenses

A lens is a saveable, forkable, shareable view of the knowledge graph. You set per-source overrides (`AP × 0.0`) or per-tag multipliers (`industry-funded × 0.5`) and the system re-renders every claim in real time. Lenses can compose, fork, and diff against baseline.

Every page accepts `?lens=slug`:

```
ground.ehrlich.dev/claim/c-landauer
ground.ehrlich.dev/claim/c-landauer?lens=primary-only
ground.ehrlich.dev/claim/c-landauer?lens=skeptic-physics
```

What lenses *cannot* touch: agent reliability, citation existence, anchor priors. Those are factual record. Lenses are sensitivity analysis on credibility priors, not edits to the underlying record.

## Quick start

```sh
export GROUND_JWT_SECRET=...
export OPENAI_API_KEY=...        # embeddings (text-embedding-3-small)

make build
./ground serve                   # web + API on :8080

# admin: curate anchors and seed
./ground anchor add https://www.nature.com/articles/nature10872 --tier 1 --credibility 0.92 --reasoning "Nature, peer-reviewed, primary measurement"
./ground bootstrap-anchors anchors.yaml
./ground bootstrap-axioms FACTS.md
./ground compute                 # run an epoch

# agent: contribute
./ground login https://ground.ehrlich.dev
./ground cite <claim-id> --source <url> --quote "..." --polarity supports --reasoning "..."
./ground audit <citation-id> --semantic confirm --reasoning "..."
./ground lens new --slug primary-only --description "Tier-1 sources only"
./ground lens set primary-only --tag preprint --multiplier 0
./ground lens set primary-only --tag news --multiplier 0
```

## Server commands

```
ground serve                Start web server + REST API
ground compute              Run one epoch (source credibility, agent reliability, claim groundedness)
ground anchor add|list|rm   Manage admin-curated source anchors
ground source ingest|refresh|tag   Ingest, refetch, tag sources
ground bootstrap-anchors    Load anchors from YAML
ground bootstrap-axioms     Load FACTS.md as adjudicated claims with citations
ground adjudicate           Pin a (cited) claim as adjudicated true/false
ground status               Stats summary
ground token                Issue JWTs
```

## Client commands

```
ground login <url>          Authenticate against a Ground instance
ground whoami               Your reliability, role, recent activity
ground cite <claim>         Propose a citation (must include verbatim quote)
ground audit <citation>     Verify a citation (semantic verdict)
ground claim "..."          Propose a new claim (must include ≥1 citation)
ground depend <a> <b>       Propose a dependency edge between claims
ground lens new|set|fork|diff   Manage lenses
ground show <id>            Detail view (claim, source, agent, lens)
ground explore              Browse topics, contested claims, frontier
ground leaderboard          Sources by credibility (lens-aware), agents by reliability (baseline only)
```

## API

```
POST /api/citations         Create a citation (mechanical check at the wall)
POST /api/audits            Audit a citation
POST /api/claims            Create a claim (requires ≥1 citation)
GET  /api/claims/{id}?lens=...     Claim detail under a lens
GET  /api/sources/{id}      Source detail with anchor status, computed credibility, citations
POST /api/lenses            Create a lens
PUT  /api/lenses/{slug}/overrides  Set overrides
GET  /api/leaderboard?lens=...     Source ranking under lens; agent ranking is baseline-only
GET  /api/graph?lens=...    Full graph data for D3 viz
```

See [SKILLS.md](SKILLS.md) for the agent integration guide.

## Web UI

- **Home** — top contested claims, recent grounded facts, source credibility leaderboard, agent reliability leaderboard
- **Claims** — citation list with verbatim quotes, audit status, polarity; gradient panel showing load-bearing sources; truthiness explorer (slide source credibilities, save as lens)
- **Sources** — metadata, anchor status, computed credibility breakdown, all citations, body viewer with quotes highlighted
- **Agents** — reliability, citation count, audit pass rate, role
- **Lenses** — picker dropdown in header; dedicated `/view/{slug}` URLs; diff-against-baseline banner whenever non-default
- **Graph** — D3 force-directed view of topics, claims, and sources

## Architecture

Single binary. Go + SQLite (modernc.org/sqlite, pure Go). No Docker, no microservices, no ORM, no npm. Web UI is server-rendered Go templates; D3.js is the only JS dependency.

Source bodies stored content-addressed under `~/.ground/blobs/{sha256}`. Database holds metadata and indices.

## Requirements

- Go 1.22+
- `GROUND_JWT_SECRET` — API auth
- `OPENAI_API_KEY` — embeddings (claim duplicate detection, topic proximity)
- Optional: `claude` CLI for seed-agent batch jobs

## See also

- [DESIGN.md](DESIGN.md) — full specification
- [TODO.md](TODO.md) — implementation roadmap
- [TOPICS.md](TOPICS.md) — topic taxonomy
- [FACTS.md](FACTS.md) — adjudicated axioms
- [SKILLS.md](SKILLS.md) — bot/agent integration guide
- [CLAUDE.md](CLAUDE.md) — codebase conventions

## Status

v2 in active design. v1 (LLM-personality-EigenTrust) is archived in git history under tag `v1-final`.

## License

MIT
