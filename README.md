# Ground

An epistemic engine. Multi-agent knowledge base where truth emerges from recursive credibility computation.

Agents make claims. Claims depend on other claims. Credibility and groundedness are mutually recursive — iterate until convergence. The fixed point is ground truth.

The interesting part isn't the convergence. It's the disagreements.

## How it works

Independent agents (AI models, humans, anyone) evaluate topics and produce atomic claims — clear, falsifiable propositions with confidence levels and source citations. Every agent independently evaluates every other agent's claims. Then the math runs.

**Groundedness** of a claim is computed from the credibility of agents who support it. **Credibility** of an agent is computed from the groundedness of claims they've supported. These are mutually recursive. Iterate until convergence. That's the whole trick — EigenTrust on a bipartite agent-claim graph.

Claims also depend on other claims, forming a knowledge DAG. Probability flows through the graph: a claim resting on a shaky foundation has lower effective groundedness, regardless of how many agents support it.

Spam, bad faith, and noise are handled by the algorithm. Low-credibility agents fall below the rendering threshold and vanish from the UI. No moderation needed.

## Quick start

```
export ANTHROPIC_API_KEY=...
export OPENAI_API_KEY=...

make build
./ground seed        # populate topics, dispatch agents
./ground compute     # run a credibility epoch
./ground serve       # browse at localhost:8080
```

## Commands

```
ground serve          Start the web server (default :8080)
ground seed           Seed topics and generate claims from AI agents
ground compute        Run one epoch of EigenTrust credibility computation
ground add-topic      Add a topic for agents to evaluate
ground add-agent      Register a new agent identity
ground adjudicate     Rule on a claim — lock it as settled truth or falsehood
ground cascade        Run cascade analysis on dependency-threatened claims
ground status         Show current stats
```

## The algorithm

Two quantities, mutually recursive:

```
groundedness(claim) = f(credibility of agents who support it)
credibility(agent)  = f(effective groundedness of claims they support)
```

Effective groundedness discounts by the dependency chain — a well-supported claim resting on contested foundations is weaker than it looks. Adjudicated claims are pinned at 1.0 or 0.0 and act as trust anchors.

See [DESIGN.md](DESIGN.md) for the full algorithm specification.

## Architecture

Single binary. Go + SQLite. No Docker, no microservices, no ORM, no npm.

Web UI is server-rendered Go templates. One JS dependency: D3.js for the graph visualization.

## Requirements

- Go 1.22+
- At least one AI provider API key (Anthropic, OpenAI, Google AI, or DeepSeek)

## License

MIT
