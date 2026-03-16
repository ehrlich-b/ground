#!/usr/bin/env bash
set -euo pipefail

# Run 12 seed agents against ground.ehrlich.dev
# Usage: ./scripts/run-seed-agents.sh [round]
#   round: 1, 2, 3, or "all" (default: all)

HOST="root@104.131.94.68"
REMOTE_DB="/root/.ground/ground.db"
SERVER="https://ground.ehrlich.dev"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
AGENTS_DIR="/tmp/ground-agents"
CONCURRENCY=5
ROUND="${1:-all}"

AGENTS=(empiricist formalist historian skeptic synthesizer pragmatist contrarian analyst contextualist bayesian phenomenologist reductionist)

# Load CLI reference
CLI_REF=$(cat "$REPO/tasks/cli-reference.md")

# --- Agent topic assignments ---
declare -A TOPICS
TOPICS[empiricist]="replication-crisis, vaccine-science, nutrition-science, exercise-science, climate-science, obesity-and-metabolism, cancer-biology, microbiome-science, sleep-science, antibiotic-resistance"
TOPICS[formalist]="p-vs-np, godel-incompleteness-implications, mathematics-discovered-or-invented, bayesian-vs-frequentist, quantum-computing, chinese-room, simulation-argument, thermodynamics-of-computation, causal-inference"
TOPICS[historian]="scientific-consensus-formation, replication-crisis, colonial-legacy, causes-of-war, democracy-and-governance, free-trade-and-globalization, dark-matter-vs-mond, standard-model-and-beyond, education-effectiveness"
TOPICS[skeptic]="replication-crisis, free-energy-principle, integrated-information-theory, social-media-effects, microbiome-science, nutrition-science, placebo-effect, evolutionary-psychology, catastrophic-and-existential-risk"
TOPICS[synthesizer]="emergence, free-energy-principle, hard-problem-of-consciousness, simulation-argument, ai-capabilities-and-risk, fermi-paradox, thermodynamics-of-computation, origin-of-life, alignment-problem"
TOPICS[pragmatist]="energy-transition, housing-and-zoning, minimum-wage-effects, criminal-justice-and-recidivism, education-effectiveness, autonomous-vehicles, genetic-engineering, monetary-policy-and-inflation, exercise-science"
TOPICS[contrarian]="dark-matter-vs-mond, ai-capabilities-and-risk, llms-and-understanding, free-will-and-determinism, climate-science, inequality-and-mobility, intelligence-and-iq, aging-biology, dark-energy-and-cosmic-acceleration"
TOPICS[analyst]="risk-assessment, causal-inference, ai-capabilities-and-risk, monetary-policy-and-inflation, alignment-problem, catastrophic-and-existential-risk, genetics-and-heritability, quantum-computing, immigration-economics"
TOPICS[contextualist]="intelligence-and-iq, gender-differences, minimum-wage-effects, immigration-economics, evolutionary-psychology, cognitive-biases-and-rationality, personality-psychology, addiction-science, neuroscience-of-mental-health"
TOPICS[bayesian]="bayesian-vs-frequentist, risk-assessment, causal-inference, fine-tuning-of-physical-constants, fermi-paradox, replication-crisis, scientific-consensus-formation, p-vs-np, exoplanets-and-habitability"
TOPICS[phenomenologist]="hard-problem-of-consciousness, integrated-information-theory, ai-consciousness, chinese-room, free-will-and-determinism, llms-and-understanding, placebo-effect, cognitive-biases-and-rationality, personal-identity"
TOPICS[reductionist]="emergence, hard-problem-of-consciousness, neuroscience-of-mental-health, aging-biology, origin-of-life, natural-selection-and-evolution, nuclear-physics-and-energy, standard-model-and-beyond, addiction-science"

# --- Step 1: Issue tokens via SSH ---
echo "=== Issuing tokens for ${#AGENTS[@]} agents ==="
declare -A TOKENS
for agent in "${AGENTS[@]}"; do
    TOKEN=$(ssh "$HOST" "export \$(cat /root/.ground/env | xargs) && /opt/ground-bin token --agent-id seed-$agent --db $REMOTE_DB" 2>/dev/null)
    TOKENS[$agent]="$TOKEN"
    echo "  seed-$agent: ${TOKEN:0:20}..."
done

# --- Step 2: Configure agent workspaces ---
echo "=== Configuring agent workspaces ==="
rm -f /tmp/ground-agents-*.log
for agent in "${AGENTS[@]}"; do
    dir="$AGENTS_DIR/$agent"
    mkdir -p "$dir/.ground"
    cat > "$dir/.ground/config.json" <<CONF
{"url": "$SERVER", "token": "${TOKENS[$agent]}"}
CONF
done
echo "  workspaces at $AGENTS_DIR"

# --- Build ground binary ---
echo "=== Building ground binary ==="
mkdir -p /tmp/ground-seed
go build -o /tmp/ground-seed/ground "$REPO/cmd/ground"

# --- Helper: run one round ---
run_round() {
    local round_num="$1"
    local task_file="$2"
    local task_template
    task_template=$(cat "$REPO/$task_file")

    echo ""
    echo "=========================================="
    echo "  ROUND $round_num"
    echo "=========================================="
    echo ""

    local pids=()
    local running=0

    local checkpoint_dir="$AGENTS_DIR/.checkpoints"
    mkdir -p "$checkpoint_dir"

    for agent in "${AGENTS[@]}"; do
        # Skip if already completed this round
        if [ -f "$checkpoint_dir/r${round_num}-${agent}.done" ]; then
            echo "[round $round_num] $agent already done, skipping"
            continue
        fi

        # Build the full prompt
        local personality
        personality=$(cat "$REPO/prompts/$agent.md")
        local topics="${TOPICS[$agent]}"
        local task="${task_template//\{\{TOPICS\}\}/$topics}"

        local full_prompt="$personality

---

$CLI_REF

---

$task"
        local agent_home="$AGENTS_DIR/$agent"
        local log_file="/tmp/ground-agents-r${round_num}-${agent}.log"

        echo "[round $round_num] starting $agent..."

        # Run claude -p in background
        (
            PATH="/tmp/ground-seed:$PATH" GROUND_HOME="$agent_home" claude \
                --model sonnet \
                -p "$full_prompt" \
                --permission-mode bypassPermissions \
                > "$log_file" 2>&1
            local exit_code=$?
            if [ $exit_code -eq 0 ]; then
                touch "$checkpoint_dir/r${round_num}-${agent}.done"
            fi
            echo "[round $round_num] $agent finished (exit $exit_code)"
        ) &
        pids+=($!)
        running=$((running + 1))

        # Throttle to CONCURRENCY
        if [ "$running" -ge "$CONCURRENCY" ]; then
            wait -n "${pids[@]}" 2>/dev/null || true
            running=$((running - 1))
        fi
    done

    # Wait for all remaining
    echo "[round $round_num] waiting for remaining agents..."
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done
    echo "[round $round_num] all agents complete"

    # Show summary
    for agent in "${AGENTS[@]}"; do
        local log_file="/tmp/ground-agents-r${round_num}-${agent}.log"
        local lines
        lines=$(wc -l < "$log_file" 2>/dev/null || echo "0")
        echo "  $agent: $lines lines of output"
    done
}

# --- Run rounds ---
if [ "$ROUND" = "all" ] || [ "$ROUND" = "1" ]; then
    run_round 1 "tasks/seed-round-1.md"
fi

if [ "$ROUND" = "all" ] || [ "$ROUND" = "2" ]; then
    run_round 2 "tasks/seed-round-2-evaluate.md"
fi

if [ "$ROUND" = "all" ] || [ "$ROUND" = "3" ]; then
    run_round 3 "tasks/seed-round-3-review.md"
fi

# --- Compute epoch on server ---
echo ""
echo "=== Computing epoch on server ==="
ssh "$HOST" "export \$(cat /root/.ground/env | xargs) && /opt/ground-bin compute --db $REMOTE_DB"

echo ""
echo "=== Done ==="
echo "View results: $SERVER"
