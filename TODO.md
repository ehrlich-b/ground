# Ground — Implementation Plan

## Phase 1: Skeleton

- [ ] `go mod init github.com/ehrlich-b/ground`
- [ ] Makefile (build with ldflags, test, clean)
- [ ] `cmd/ground/main.go` — Cobra root command + version
- [ ] Subcommand stubs: serve, seed, compute, add-topic, add-agent, adjudicate, cascade, status
- [ ] `internal/model/` — all data types (Agent, Topic, Claim, Assertion, Dependency, Epoch)

## Phase 2: Database

- [ ] `internal/db/db.go` — Open(), WAL mode, foreign keys, migration runner
- [ ] `internal/db/migrations/001_init.sql` — full schema from DESIGN.md
- [ ] CRUD methods on Store: agents, topics, claims, assertions, dependencies, epochs
- [ ] Helper scan functions for each model type
- [ ] Unit tests for Store methods

## Phase 3: Embeddings

- [ ] `internal/embed/embed.go` — interface for embedding generation
- [ ] OpenAI embedding adapter (text-embedding-3-small or similar)
- [ ] Cosine similarity function
- [ ] Topic exclusion check — compare against exclusion anchors
- [ ] Claim-to-topic proximity — find nearest topic anchors for a claim
- [ ] Seed exclusion anchors (curated list, embedded in code or migration)

## Phase 4: AI Agent Dispatch

- [ ] `internal/agent/agent.go` — interface: GenerateClaims(topic), EvaluateClaims(topic, claims)
- [ ] Prompt templates for claim generation and evaluation
- [ ] Claude adapter (Anthropic API)
- [ ] GPT adapter (OpenAI API)
- [ ] Gemini adapter (Google AI API)
- [ ] DeepSeek adapter (DeepSeek API)
- [ ] Response parsing — JSON claims/evaluations into model types
- [ ] Handle refine stance — create child claims, link parent
- [ ] Handle dependency extraction — agents identify which existing claims a new claim depends on

## Phase 5: EigenTrust Engine

- [ ] `internal/engine/engine.go` — the core iteration loop
- [ ] Step 1: Intrinsic groundedness from credibility-weighted assertions
- [ ] Step 2: Effective groundedness via dependency DAG (topological sort)
- [ ] Step 3: Credibility from effective groundedness
- [ ] Skip adjudicated claims in step 1
- [ ] Damping factor implementation
- [ ] Convergence check (max delta < epsilon or max iterations)
- [ ] Contestation score computation
- [ ] Claim status transitions (active -> contested/emerging -> grounded/refuted)
- [ ] Stability tracking across epochs
- [ ] Persist results — update agents.credibility, claims.groundedness, claims.effective_groundedness, claims.status
- [ ] Create epoch record with iteration count and final delta
- [ ] Unit tests with hand-crafted assertion graphs and known fixed points

## Phase 6: CLI Commands

- [ ] `ground seed` — insert 20 topics, register AI agents, dispatch claim generation, dispatch cross-evaluation
- [ ] `ground compute` — run one EigenTrust epoch, print summary
- [ ] `ground add-topic` — add topic by title/description, generate embedding, check exclusions
- [ ] `ground add-agent` — register agent with name/metadata
- [ ] `ground adjudicate` — set claim as adjudicated true/false with reasoning
- [ ] `ground cascade` — find dependency-threatened claims, print tree, optionally re-dispatch agents
- [ ] `ground status` — print agent count, claim count, epoch count, top grounded, top contested
- [ ] `ground serve` — start HTTP server

## Phase 7: Web UI — Templates & Handlers

- [ ] `internal/web/server.go` — Server struct, route registration, template loading
- [ ] Base layout template (dark mode, monospace headers, clean sans-serif body)
- [ ] CSS — "terminal aesthetics meets academic rigor"
- [ ] Home page (`/`) — hero, top contested claims, top grounded claims, agent leaderboard
- [ ] Topic page (`/topic/{slug}`) — claims sorted by groundedness, agent stance breakdown per claim
- [ ] Agent page (`/agent/{id}`) — credibility score, claim history, accuracy track record
- [ ] Claim page (`/claim/{id}`) — all assertions with reasoning/sources, dependency tree, effective vs intrinsic groundedness, refinement chain
- [ ] About page (`/about`) — philosophy, algorithm explanation, "truth as a fixed point"

## Phase 8: Graph Visualization

- [ ] Graph page (`/graph`) — D3.js force-directed graph
- [ ] Topic anchors as large nodes, claims as smaller nodes orbiting by proximity
- [ ] Edges for assertions (agent -> claim) colored by stance
- [ ] Edges for dependencies (claim -> claim)
- [ ] Node size/color by groundedness
- [ ] Agent nodes sized by credibility
- [ ] Interactive — click to navigate to detail pages

## Phase 9: Truthiness Explorer

- [ ] Interactive dependency tree on claim pages
- [ ] Toggle assumptions on/off — recalculate effective groundedness client-side
- [ ] Slider for dependency groundedness — "what if this were more/less certain?"
- [ ] Show gap between intrinsic and effective groundedness

## Phase 10: Polish & Ship

- [ ] Screenshot-friendly layouts for X/Twitter (og:image meta, clean card previews)
- [ ] .gitignore (ground.db, .env, etc.)
- [ ] README final pass
- [ ] Deploy to ground.ehrlich.dev
