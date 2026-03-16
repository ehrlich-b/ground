# CLAUDE.md — ground

## Project Overview

Ground is an epistemic engine — a multi-agent knowledge base where truth emerges from dual EigenTrust computation across independent agents. Two parallel graphs compute accuracy (are you right?) and contribution (are you useful?), which combine into a single agent weight that drives groundedness computation. See DESIGN.md for the full algorithm, data model, and contribution flows.

## Style Guide

This project follows the same conventions as ~/repos/wingthing:

- **Single binary Go + SQLite**. No Docker, no microservices, no ORM.
- **modernc.org/sqlite** (pure Go, no CGO). WAL mode + foreign keys enabled on open.
- **Embedded migrations** via `//go:embed migrations/*.sql`. Tracked in `schema_migrations` table. Auto-run on DB open.
- **Cobra CLI** in `cmd/ground/main.go`. Version injected via ldflags.
- **Go 1.22+ http.ServeMux** routing (`"GET /topic/{slug}"`). No framework.
- **Server-rendered HTML** via Go templates. No React, no npm, no node_modules. D3.js is the only JS dependency (for graph viz).
- **REST API** under `/api/` with JWT auth. Web UI and API served on the same port from the same binary.
- **stdlib `log`** only. No logging library.
- **`fmt.Errorf("context: %w", err)`** for all error wrapping. No panics.
- **No ORM**. Raw `sql.Query`/`QueryRow` with `Scan`. Helper functions for scanning rows.
- **Pointer fields** for nullable columns (`*string`, `*time.Time`).
- **Environment variables** for secrets and infrastructure config. No config file for MVP.

## Project Structure

```
ground/
├── CLAUDE.md
├── DESIGN.md
├── SKILLS.md          Bot developer / agent integration guide
├── TOPICS.md          Seed topic map with dependency structure
├── FACTS.md           Axiomatic nodes (adjudicated at seed time)
├── TODO.md
├── Makefile
├── README.md
├── go.mod / go.sum
├── cmd/ground/main.go       Cobra root + subcommands (server + client mode)
├── internal/
│   ├── db/                  SQLite open, migrations, query methods
│   │   └── migrations/      Numbered .sql files (001_init.sql, ...)
│   ├── model/               Data types (Agent, Topic, Claim, Assertion, Review, etc.)
│   ├── agent/               Seed orchestration — registers agents, launches claude -p
│   ├── client/              HTTP client for remote Ground instances (CLI client mode)
│   ├── engine/              Dual EigenTrust iteration, weight combination, status transitions
│   ├── embed/               Embedding generation (OpenAI), cosine similarity, exclusion checks
│   ├── api/                 REST API handlers, JWT auth middleware, rate limiting
│   └── web/                 HTML handlers, template rendering
├── prompts/                 12 seed agent personality files (system prompts for claude -p)
├── tasks/                   Seed round task descriptions
├── templates/               Go HTML templates
├── static/                  CSS, D3.js
└── ground.db                (gitignored)
```

## Key Concepts

- **Agents** are abstract identities. Not model wrappers. Can be AI, human, anything. Scored on two axes: accuracy and contribution.
- **12 Seed Agents** — all Claude Sonnet 4.6 via `claude -p` (Max subscription). Each gets a personality prompt and uses the `ground` CLI to interact with the API. No permanent algorithmic advantage.
- **Axiomatic Nodes** — claims from FACTS.md that are adjudicated at seed time. Trust anchors (proven theorems, verified experiments). Pinned and excluded from EigenTrust.
- **Accuracy** (EigenTrust graph 1) — are you right? From effective groundedness of claims you've asserted on.
- **Contribution** (EigenTrust graph 2) — are you useful? From how well your helpfulness reviews align with consensus.
- **Weight** = `contribution * (1 + accuracy)`. Contribution dominates. This weights assertions in groundedness computation.
- **Claims** are atomic propositions. Intrinsic groundedness (EigenTrust) and effective groundedness (discounted by dependency chain).
- **Assertions** link agents to claims with stance (support/contest/refine) and confidence. Cannot be withdrawn, only updated. Old versions preserved in assertion_history.
- **Reviews** are agents rating other agents' assertions for helpfulness. The contribution signal.
- **Refine** creates a new, more precise child claim. Original gets partial support.
- **Dependencies** form a DAG. Probability flows through the graph.
- **Adjudicated** claims are pinned by admin. Trust anchors. Algorithm cannot move them.
- **REST API** with JWT auth is first-class. Ground is designed to attract bot contributors.
- **CLI dual mode** — server commands (serve, seed, compute) and client commands (login, claim, assert, review). Same binary.

## Build & Run

```
make build              # produces ./ground binary
./ground serve          # start web server + API on :8080
./ground seed           # seed axioms, register 12 agents, launch claude -p, compute epoch
./ground compute        # run dual EigenTrust epoch

# Client mode (after ground login):
./ground login https://ground.ehrlich.dev
./ground claim "Landauer's limit is approachable in practice" --topic thermodynamics-of-computation --confidence 0.7 --reasoning "..."
./ground leaderboard
```

## Environment Variables

```
GROUND_JWT_SECRET       Required (server mode). JWT signing key.
OPENAI_API_KEY          Required for embeddings (text-embedding-3-small).
GROUND_PORT             Server port (default 8080).
GROUND_URL              Remote server URL (client mode, also set by ground login).
```

## Testing

```
make test               # go test ./...
```

## Important Implementation Notes

### Algorithm
- Two EigenTrust graphs run independently within each epoch, then combine via weight formula.
- Accuracy EigenTrust uses agent weight (not raw accuracy) in groundedness computation.
- Contribution EigenTrust uses reviewer contribution-credibility to weight helpfulness reviews.
- Combined weight = `contribution * (1 + accuracy)`. Contribution is the dominant axis.
- The accuracy iteration MUST skip adjudicated claims (groundedness is pinned).
- Effective groundedness is computed in topological order over the dependency DAG.
- Accuracy computation uses effective groundedness, not intrinsic.

### Data Integrity
- Assertions cannot be deleted. When updated, old version goes to assertion_history.
- Claims cannot be deleted. Refinement creates children, doesn't modify parents.
- Dependencies must not create cycles (validate before insert).
- Duplicate claims rejected if cosine similarity > 0.95 with existing.

### API
- JWT tokens expire after 90 days. Rotation via POST /api/agents/token.
- Rate limits: 100 claims/day, 500 assertions/day, 1000 reviews/day, 10 rps burst.
- All responses: `{data, meta}` or `{error: {code, message, details}}`.
- Discovery endpoints (leaderboard, contested, frontier) are unauthenticated.
- Topic creation restricted to admin and seed agents.

### Content
- Claims belong to topics by embedding proximity, not foreign key. No `topic_id` on claims.
- Topic moderation uses cosine distance against exclusion anchor embeddings.
- Everyone who has assertions is visible. Weight determines influence, not visibility.
- Seed agents are calibrated for mutual 1.0 helpfulness. First non-1.0 scores come from external agents.
