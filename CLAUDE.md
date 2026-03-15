# CLAUDE.md — ground

## Project Overview

Ground is an epistemic engine — a multi-agent knowledge base where truth emerges from recursive credibility computation (EigenTrust) across independent agents. See DESIGN.md for the full algorithm and data model specification.

## Style Guide

This project follows the same conventions as ~/repos/wingthing:

- **Single binary Go + SQLite**. No Docker, no microservices, no ORM.
- **modernc.org/sqlite** (pure Go, no CGO). WAL mode + foreign keys enabled on open.
- **Embedded migrations** via `//go:embed migrations/*.sql`. Tracked in `schema_migrations` table. Auto-run on DB open.
- **Cobra CLI** in `cmd/ground/main.go`. Version injected via ldflags.
- **Go 1.22+ http.ServeMux** routing (`"GET /topic/{slug}"`). No framework.
- **Server-rendered HTML** via Go templates. No React, no npm, no node_modules. D3.js is the only JS dependency (for graph viz).
- **stdlib `log`** only. No logging library.
- **`fmt.Errorf("context: %w", err)`** for all error wrapping. No panics.
- **No ORM**. Raw `sql.Query`/`QueryRow` with `Scan`. Helper functions for scanning rows.
- **Pointer fields** for nullable columns (`*string`, `*time.Time`).
- **Environment variables** for API keys and infrastructure config. No config file needed for MVP.

## Project Structure

```
ground/
├── CLAUDE.md
├── DESIGN.md
├── TODO.md
├── Makefile
├── README.md
├── go.mod / go.sum
├── cmd/ground/main.go       Cobra root + subcommands
├── internal/
│   ├── db/                  SQLite open, migrations, query methods
│   │   └── migrations/      Numbered .sql files (001_init.sql, ...)
│   ├── model/               Data types (Agent, Topic, Claim, Assertion, etc.)
│   ├── agent/               AI agent dispatch, prompt templates, response parsing
│   ├── engine/              EigenTrust iteration, contestation scoring, status transitions
│   ├── embed/               Embedding generation (OpenAI/etc), cosine similarity, exclusion checks
│   └── web/                 HTTP server, handlers, template rendering
├── templates/               Go HTML templates
├── static/                  CSS, D3.js
└── ground.db                (gitignored)
```

## Key Concepts

- **Agents** are abstract identities with a credibility score. Not model wrappers. Can be AI, human, anything.
- **Claims** are atomic propositions. They have intrinsic groundedness (from EigenTrust) and effective groundedness (discounted by dependency chain).
- **Assertions** link agents to claims with a stance (support/contest/refine) and confidence.
- **Refine** creates a new, more precise child claim. The original gets partial support (0.3 * confidence).
- **Dependencies** form a DAG between claims. Probability flows through the graph.
- **Adjudicated** claims are pinned by admin — the algorithm cannot move them. They're trust anchors.
- **Epochs** are computation runs. Track iterations to convergence and final delta.

## Build & Run

```
make build              # produces ./ground binary
./ground seed           # seed topics + dispatch AI agents
./ground compute        # run EigenTrust epoch
./ground serve          # start web server on :8080
```

## API Keys (env vars)

```
ANTHROPIC_API_KEY       Claude
OPENAI_API_KEY          GPT + embeddings
GOOGLE_AI_API_KEY       Gemini
DEEPSEEK_API_KEY        DeepSeek
```

## Testing

```
make test               # go test ./...
```

## Important Implementation Notes

- The EigenTrust iteration MUST skip adjudicated claims (groundedness is pinned).
- Effective groundedness is computed in topological order over the dependency DAG.
- Credibility computation uses effective groundedness, not intrinsic.
- Agents below `render_threshold` (0.2) still participate in computation but are hidden from UI.
- Topic moderation uses cosine distance against exclusion anchor embeddings, not keyword matching.
- Claims belong to topics by embedding proximity, not foreign key. There is no `topic_id` on claims.
