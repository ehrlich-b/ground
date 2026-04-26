# Ground — Implementation Plan (v2 reboot)

The v1 codebase (Go binary, SQLite, REST API, JWT auth, web UI, D3 graph viz, dual-mode CLI, fanout harness) is largely intact and worth keeping. v2 reuses the infrastructure and replaces the *signal layer*: assertions and reviews are gone; citations and audits take their place. Lenses are added as a first-class concept.

Phases are ordered by dependency. Each phase should ship behind a `make build` that passes `go test ./...`.

---

## Phase 0: Reboot prep

- [ ] Tag v1: `git tag v1-final`, push tag
- [ ] Snapshot the v1 ground.db elsewhere; `rm ground.db ground.db-shm ground.db-wal`
- [ ] Move v1-specific docs to `docs/v1/` (DESIGN.md history etc.) — actually skip; keep git history as the archive
- [ ] Create `docs/` directory for any v2 supplementary notes (anchor curation policy, lens examples)
- [ ] Wipe `tasks/` directory; v1 task prompts are no longer applicable

## Phase 1: Schema migration

- [ ] `internal/db/migrations/002_v2_schema.sql`
  - [ ] `sources` (id, url, content_hash, body_blob_id, fetched_at, type, metadata)
  - [ ] `source_anchors` (source_id, credibility, set_by, reasoning, set_at)
  - [ ] `source_tags` (source_id, tag) — composite PK
  - [ ] `source_credibility` (source_id, epoch_id, value, components_json)
  - [ ] `citations` (id, claim_id, source_id, verbatim_quote, locator_json, polarity, extractor_id, created_at)
  - [ ] `audits` (id, citation_id, auditor_id, mechanical, semantic, verdict, reasoning, created_at)
  - [ ] `lenses` (id, slug, owner_id, parent_lens_id, description, created_at)
  - [ ] `lens_overrides` (lens_id, source_id, mode, value)
  - [ ] `lens_tag_overrides` (lens_id, tag, multiplier)
  - [ ] Add `agents.role`, `agents.reliability`, `agents.productivity`. Leave v1 score columns nullable; do not drop yet.
  - [ ] Indexes: `(source_id)` on citations, `(citation_id)` on audits, `(claim_id, polarity)` on citations
- [ ] `internal/model/` — new types: `Source`, `Citation`, `Audit`, `Lens`, `LensOverride`, `SourceTag`, `SourceAnchor`, `SourceCredibility`
- [ ] `internal/db/` — CRUD methods for all new tables
- [ ] Helper: `Store.HasSourceQuote(source_id, quote string) (bool, error)` for the mechanical check
- [ ] Unit tests for new Store methods

## Phase 2: Source ingestion

- [ ] `internal/sources/` — new package
- [ ] `Fetcher` interface; default implementation does HTTP GET with timeout, content-type sniff
  - [ ] HTML → readability extraction (use `github.com/go-shiori/go-readability` or roll-your-own)
  - [ ] PDF → text extraction (use `github.com/dslipak/pdf` or shell out to `pdftotext`)
  - [ ] arXiv abs/PDF special-casing
  - [ ] PubMed Central XML special-casing
  - [ ] OpenAlex API helper for metadata enrichment
- [ ] Body blob storage: filesystem at `~/.ground/blobs/{sha256}` for MVP; abstract behind `BlobStore` interface for future S3
- [ ] `Source` content-hash dedup: same content_hash → same source_id, even across URLs
- [ ] Refetch policy: `ground refresh-sources` command; updates `fetched_at`, content_hash; flags drift
- [ ] Handle paywalls/JS-walls explicitly: if body length below threshold or detected paywall pattern, set `type=unverifiable` and credibility floor
- [ ] Auto-tag on ingest: `peer-reviewed` (DOI + journal in allowlist), `preprint` (arXiv), `news` (host in news list), `government` (.gov), `wiki` (wikipedia.org), `blog` (catch-all)
- [ ] Citation graph extraction: parse references from PDFs/HTML where feasible; persist source-source edges
- [ ] Tests with golden source fixtures in `testdata/sources/`

## Phase 3: Citations and the mechanical wall

- [ ] `POST /api/citations` handler
  - [ ] Resolve source: by `source_id` if provided, else by `url` (fetch if not cached)
  - [ ] **Mechanical check**: `strings.Contains(source.body, citation.verbatim_quote)` — return 400 with explicit error if fails
  - [ ] Persist citation with provisional `audit_factor=1.0`
  - [ ] Enqueue audit job
- [ ] `GET /api/citations/{id}` — includes audit list
- [ ] `GET /api/claims/{id}/citations`
- [ ] `internal/api/citations.go` handlers
- [ ] CLI: `ground cite <claim-id> --source <url-or-id> --quote "..." --polarity supports --reasoning "..."`
- [ ] Citation locator schema: `{type: "char_offset", offset: int, length: int}` for plaintext; `{type: "page", page: int, line_hint: string}` for PDFs; locators are advisory, the verbatim quote is authoritative
- [ ] Tests: hallucinated quote rejected; valid quote persisted; idempotent on re-post

## Phase 4: Audits

- [ ] `POST /api/audits` handler
  - [ ] Auditor cannot audit own citations (check `extractor_id != auditor_id`)
  - [ ] Re-run mechanical check; persist `mechanical` verdict
  - [ ] Accept semantic verdict + reasoning from caller (LLM agent does the reading)
  - [ ] Combine into `verdict` (uphold/reject)
- [ ] `GET /api/citations/{id}/audits`
- [ ] CLI: `ground audit <citation-id> --semantic confirm --reasoning "..."`
- [ ] Audit assignment helper: `GET /api/audits/queue?agent_id=...&limit=10` returns citations needing audit, weighted by reliability-aware sampling (agents with higher reliability get harder/more contested citations)
- [ ] Drift handler: when `Source.refetch` finds the body changed, re-run mechanical checks on all that source's citations; flip mechanical to `fail` where the quote no longer appears
- [ ] Tests: self-audit rejected; mechanical-fail forces verdict=reject regardless of semantic

## Phase 5: Engine — epoch v2

Replace `internal/engine/engine.go` (or rewrite into `engine_v2.go` and feature-flag).

- [ ] Source credibility iteration
  - [ ] Load anchors as priors
  - [ ] EigenTrust over source citation graph (source → source edges)
  - [ ] Audit aggregate: avg uphold rate of citations *of* this source
  - [ ] Combine with `(w_anchor, w_graph, w_audit)` weights
  - [ ] Persist `source_credibility(epoch_id, source_id, value, components)`
- [ ] Agent reliability iteration
  - [ ] EigenTrust over audit verdicts: auditor_reliability weights uphold/reject
  - [ ] Mechanical-fail = hard reject regardless of auditor
  - [ ] Damping toward prior; agents with no audited work stay at prior
  - [ ] Persist `agents.reliability`
- [ ] Claim intrinsic groundedness
  - [ ] Linear combination over citations: `w_c = credibility(source) * reliability(extractor) * audit_factor(citation)`
  - [ ] `audit_factor`: weighted uphold ratio; 0 if any mechanical-fail; 1 if unaudited (provisional)
  - [ ] Polarity coefficients: `+1 supports, -alpha contradicts, +beta qualifies`
  - [ ] Adjudicated claims pinned, skipped
- [ ] Effective groundedness via topo DAG (preserve v1 logic)
- [ ] Contestation: variance of credibility-weighted polarities (not assertion stances)
- [ ] Status transitions (preserve v1 thresholds, configurable)
- [ ] Persist `claims.groundedness`, `claims.effective_groundedness`, `claims.status`, `claims.contestation`
- [ ] Epoch record with iteration counts and deltas
- [ ] Hand-crafted tests with known fixed points

## Phase 6: Lens engine (cheap render)

- [ ] `internal/lens/` — new package
- [ ] `Renderer` struct: holds in-memory baseline (sources, citations, deps, agent reliability)
- [ ] `Renderer.LoadEpoch(epoch_id)` — pull baseline from DB once
- [ ] `Renderer.Render(lens) -> map[claim_id]ClaimScore` — sparse merge, linear pass, topo pass
  - [ ] Implement merge: per-source override → per-tag multiplier → fall through to baseline
  - [ ] Cache results keyed by `(lens_id, epoch_id)`
- [ ] `Renderer.RenderClaim(lens, claim_id) -> ClaimScore` for single-claim views
- [ ] `Renderer.Gradient(claim_id) -> []SourceImpact` for "load-bearing sources" view
- [ ] Benchmark: target sub-100ms for 10k claims, sub-second for 100k
- [ ] Tests: lens override matches baseline when empty; mass discount of a tag affects all sources with that tag; gradients sign-correct

## Phase 7: Lens API + saved lenses

- [ ] `POST /api/lenses` — create lens
- [ ] `GET /api/lenses/{slug}` — fetch lens definition
- [ ] `PUT /api/lenses/{slug}/overrides` — bulk set
- [ ] `POST /api/lenses/{slug}/fork` — fork into a new lens
- [ ] All read endpoints accept `?lens=slug` query parameter
  - [ ] `GET /api/claims/{id}?lens=...`
  - [ ] `GET /api/leaderboard?lens=...` — applies to source ranking; agent rankings ignore lens
  - [ ] `GET /api/contested?lens=...`
  - [ ] `GET /api/graph?lens=...`
- [ ] CLI: `ground lens new`, `ground lens set <source> <value>`, `ground lens fork`, `ground lens diff baseline`
- [ ] Lens permissions: lens is owned by an agent; only owner can edit; forking is open
- [ ] Tests: lens query param threads through all endpoints; agent reliability never moves under a lens

## Phase 8: Server CLI v2

- [ ] `ground anchor add <url> --tier 1 --credibility 0.95 --reasoning "..."` — admin-only
- [ ] `ground anchor list`, `ground anchor remove`
- [ ] `ground source ingest <url>` — fetch and store, return source_id
- [ ] `ground source refresh [--all | --source <id>]` — re-fetch, drift detect, re-mechanical-check
- [ ] `ground source tag <source-id> <tag>`
- [ ] `ground compute` — replace v1 epoch with v2 epoch
- [ ] `ground status` — agents by reliability, claims by status, sources by credibility, recent audit failures
- [ ] `ground adjudicate` — same as v1 but now requires citations on the adjudicated claim
- [ ] Removed: `ground review`, v1 `ground assert` semantics; deprecation warning if invoked
- [ ] `ground cite`, `ground audit` — agent-facing CLI commands

## Phase 9: Web UI v2

- [ ] Source pages: `/source/{id}` — metadata, citations, anchor status, computed credibility breakdown, "set lens override here" widget
- [ ] Sources index: `/sources` — searchable, filterable by tag, sortable by credibility
- [ ] Claim page: replace assertion list with citation list. Each citation shows source, verbatim quote (highlighted in cached body view on click), polarity, audit status (3/3 uphold etc.)
- [ ] Lens picker in header: dropdown of saved lenses, "create new" affordance
- [ ] Lens diff banner: when `?lens=` is non-default, sticky banner shows "differs from baseline on N sources, top affected: ..."
- [ ] Truthiness explorer v2: per-claim sliders for top-N source credibilities (gradients-driven), saved as a lens with one click
- [ ] Agent page: reliability, citation count, audit pass rate, recent extractions, recent audits
- [ ] Anchor curation page: `/anchors` — read-only list of all anchored sources with reasoning; admin link to add/edit
- [ ] Source body viewer: `/source/{id}/body` — cached text with citation quotes highlighted

## Phase 10: Graph viz v2

- [ ] D3 graph nodes now include sources (third type alongside topics and claims)
- [ ] Edge types: claim→source (citation, colored by polarity), claim→claim (dependency), source→source (citation graph)
- [ ] Filter UI: hide/show source layer; filter by tag; filter by anchor tier
- [ ] Lens-aware coloring: claim node color reflects lens-rendered groundedness, not baseline
- [ ] Click source → source page; click claim → claim page

## Phase 11: Bootstrapping content

- [ ] Hand-curate ~100 anchor sources covering the topic taxonomy. Commit `anchors.yaml` to repo; loaded by `ground bootstrap-anchors`
- [ ] Rewrite FACTS.md axioms to include verbatim quotes from anchored sources
- [ ] `ground bootstrap-axioms` — parses FACTS.md, creates adjudicated claims with citations
- [ ] Initial source ingestion: ~500 sources covering Tier 1 anchors via DOI/arXiv ID lists
- [ ] Manual smoke test: pick 5 axioms, verify their citations pass mechanical check end to end

## Phase 12: Seed agent orchestration v2

Reuse `scripts/run-seed-agents.sh` structure; replace task prompts.

- [ ] `tasks/v2-search.md` — given a topic, find candidate sources (URLs + reasoning)
- [ ] `tasks/v2-extract.md` — given a claim and a source, propose citations with verbatim quotes
- [ ] `tasks/v2-audit.md` — given a citation, audit it (semantic verdict + reasoning)
- [ ] `tasks/v2-deps.md` — given claims, propose dependency edges (preserved from v1, minor cleanup)
- [ ] Personality prompts repurposed: each personality biases search and extraction style, not stance
- [ ] Remove: contest quotas, forced-stance logic, helpfulness ratings
- [ ] New round order: search → ingest → extract → audit → deps → compute
- [ ] Audit assignment is reliability-weighted random, not all-pairs

## Phase 13: Cleanup and ship

- [ ] `internal/db/migrations/003_drop_v1.sql` — drop `assertions`, `reviews`, `assertion_history`; drop `agents.accuracy`, `agents.contribution`, `agents.weight` columns
- [ ] Remove dead code: v1 engine, v1 assertion/review handlers, v1 prompt-personality contest logic
- [ ] Remove v1 client commands: `ground assert`, `ground review`
- [ ] README final pass — v2 framing
- [ ] SKILLS.md final pass — extract/audit/search guide
- [ ] Twitter/X card meta tags updated for v2 framing
- [ ] Deploy to ground.ehrlich.dev; smoke test under live domain
- [ ] Tag `v2.0`

---

## What's deliberately NOT here

- **No multi-model voting.** No "average across N LLMs" stance aggregation. Disagreement comes from real source contradictions, not synthetic agent diversity.
- **No reasoning-chain validation.** v2 is an encyclopedia of weighted facts and their citation backing, not a logic system. If you want "this argument is valid," that's a different project.
- **No real-time fetching during request.** All fetches async; UI shows cached state.
- **No federation between Ground instances.** Lenses are the federation primitive at the user level.

## Open design questions

- [ ] Audit consensus: 3 audits enough? Should disputed citations (1 uphold, 2 reject) trigger a 4th tiebreaker or just stay at low audit_factor?
- [ ] Lens visibility: are all lenses public by default, or owner-private with explicit publish? Suggest: public by default, "draft" flag for private experimentation.
- [ ] Anchor governance: who is admin? For now Bryan only. Eventually a small admin set with PR-based proposals?
- [ ] Should agents have separate reliability scores per topic? Probably not — keep one global reliability and let the audit signal speak. Revisit if topic-specific dishonesty emerges.
- [ ] Wikipedia tier: tier 2 default seems right (secondary source, generally well-edited, citations to primary). User lenses can shift it. Anchor list should NOT include individual Wikipedia articles — too many to curate.
