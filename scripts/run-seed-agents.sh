#!/usr/bin/env bash
set -euo pipefail

# Seed agent harness for ground.ehrlich.dev
# Usage: ./scripts/run-seed-agents.sh [round]
#   round: dep, 2, 3, or "all" (default: all)

HOST="root@104.131.94.68"
REMOTE_DB="/root/.ground/ground.db"
SERVER="https://ground.ehrlich.dev"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
WORK_DIR="/tmp/ground-agents"
CONCURRENCY=5
ROUND="${1:-all}"

AGENTS=(empiricist formalist historian skeptic synthesizer pragmatist contrarian analyst contextualist bayesian phenomenologist reductionist)

# --- Agent topic assignments (slugs match server) ---
declare -A TOPICS
TOPICS[empiricist]="sciences-self-correction-problem, immune-priming-and-disease-prevention, what-we-know-about-diet-and-health, movement-and-health, the-physics-of-warming, the-obesity-epidemic, what-we-know-about-cancer, the-gut-brain-immune-axis, what-we-know-and-dont-know-about-sleep, the-post-antibiotic-threat"
TOPICS[formalist]="the-most-important-open-problem-in-computer-science, what-incompleteness-actually-means, the-ontological-status-of-mathematical-objects, frameworks-for-reasoning-under-uncertainty, capabilities-and-limits-of-quantum-computation, can-syntax-produce-semantics, are-we-living-in-a-simulation, energy-costs-of-information-processing, how-to-know-if-x-causes-y"
TOPICS[historian]="how-science-decides-whats-true, sciences-self-correction-problem, the-long-shadow-of-empire, why-states-fight, how-political-systems-perform, who-benefits-from-trade, the-missing-mass-problem, what-particle-physics-knows-and-doesnt, what-works-in-education"
TOPICS[skeptic]="sciences-self-correction-problem, the-brain-as-a-prediction-machine, iit-as-a-theory-of-consciousness, what-screens-are-doing-to-us, the-gut-brain-immune-axis, what-we-know-about-diet-and-health, when-belief-becomes-biology, evolved-behavioral-adaptations, threats-to-civilization"
TOPICS[synthesizer]="emergence-strong-vs-weak, the-brain-as-a-prediction-machine, why-anything-feels-like-anything, are-we-living-in-a-simulation, what-ai-can-do-now-and-where-its-going, where-is-everybody, energy-costs-of-information-processing, how-life-began, making-ai-do-what-we-actually-want"
TOPICS[pragmatist]="moving-from-fossil-fuels, why-housing-is-expensive, what-happens-when-you-raise-the-minimum-wage, what-reduces-crime, what-works-in-education, self-driving-cars, crispr-and-biological-design, what-causes-inflation-and-how-central-banks-respond, movement-and-health"
TOPICS[contrarian]="the-missing-mass-problem, what-ai-can-do-now-and-where-its-going, do-language-models-understand, do-we-choose, the-physics-of-warming, the-gap-between-rich-and-poor, what-iq-measures-and-what-it-predicts, why-we-age-and-whether-we-must, the-accelerating-universe"
TOPICS[analyst]="how-to-think-about-danger, how-to-know-if-x-causes-y, what-ai-can-do-now-and-where-its-going, what-causes-inflation-and-how-central-banks-respond, making-ai-do-what-we-actually-want, threats-to-civilization, what-genes-explain-and-dont-explain, capabilities-and-limits-of-quantum-computation, economic-effects-of-immigration"
TOPICS[contextualist]="what-iq-measures-and-what-it-predicts, nature-nurture-and-the-gap, what-happens-when-you-raise-the-minimum-wage, economic-effects-of-immigration, evolved-behavioral-adaptations, systematic-errors-in-human-reasoning, the-big-five-and-beyond, compulsive-use-despite-harm, what-brain-science-knows-about-psychiatric-disorders"
TOPICS[bayesian]="frameworks-for-reasoning-under-uncertainty, how-to-think-about-danger, how-to-know-if-x-causes-y, why-the-constants-have-the-values-they-do, where-is-everybody, sciences-self-correction-problem, how-science-decides-whats-true, the-most-important-open-problem-in-computer-science, other-worlds"
TOPICS[phenomenologist]="why-anything-feels-like-anything, iit-as-a-theory-of-consciousness, could-machines-be-conscious, can-syntax-produce-semantics, do-we-choose, do-language-models-understand, when-belief-becomes-biology, systematic-errors-in-human-reasoning, what-makes-you-you-over-time"
TOPICS[reductionist]="emergence-strong-vs-weak, why-anything-feels-like-anything, what-brain-science-knows-about-psychiatric-disorders, why-we-age-and-whether-we-must, how-life-began, the-central-framework-of-biology, nuclear-fission-fusion-and-radiation, what-particle-physics-knows-and-doesnt, compulsive-use-despite-harm"

# --- Setup ---
mkdir -p "$WORK_DIR/.checkpoints"

# --- Step 1: Issue tokens ---
echo "=== Issuing tokens for ${#AGENTS[@]} agents ==="
declare -A TOKENS
for agent in "${AGENTS[@]}"; do
    TOKEN=$(ssh "$HOST" "export \$(cat /root/.ground/env | xargs) && /opt/ground-bin token --agent-id seed-$agent --db $REMOTE_DB" 2>/dev/null)
    TOKENS[$agent]="$TOKEN"
    echo "  seed-$agent: ${TOKEN:0:20}..."
done

# --- Dependency Mapping Round ---
run_round_dep() {
    echo ""
    echo "=========================================="
    echo "  DEPENDENCY MAPPING ROUND"
    echo "=========================================="

    local pids=()
    local running=0

    for agent in "${AGENTS[@]}"; do
        if [ -f "$WORK_DIR/.checkpoints/dep-${agent}.done" ]; then
            echo "[dep] $agent: already done, skipping"
            continue
        fi

        echo "[dep] starting $agent..."

        (
            agent_name="$agent"
            log="/tmp/ground-agents-dep-${agent_name}.log"
            token="${TOKENS[$agent_name]}"
            topics="${TOPICS[$agent_name]}"
            personality=$(cat "$REPO/prompts/${agent_name}.md")
            task_template=$(cat "$REPO/tasks/seed-round-dep-map.md")
            task_template="${task_template//\{\{TOPICS\}\}/$topics}"
            checkpoint_file="$WORK_DIR/.checkpoints/dep-${agent_name}-topics.txt"

            echo "$(date +%H:%M:%S) starting dep round for $agent_name" > "$log"

            AGENT_TOPICS="$topics" PERSONALITY="$personality" \
            TASK_TEMPLATE="$task_template" API_SERVER="$SERVER" API_TOKEN="$token" \
            AGENT_NAME="$agent_name" CHECKPOINT_FILE="$checkpoint_file" \
            python3 << 'PYEOF'
import os, json, re, subprocess, sys, urllib.request, urllib.error

agent_topics = os.environ["AGENT_TOPICS"]
personality = os.environ["PERSONALITY"]
task_template = os.environ["TASK_TEMPLATE"]
server = os.environ["API_SERVER"]
token = os.environ["API_TOKEN"]
agent_name = os.environ["AGENT_NAME"]
checkpoint_file = os.environ["CHECKPOINT_FILE"]

BATCH_SIZE = 8

# Load checkpoint (set of already-processed topic slugs)
done_topics = set()
try:
    with open(checkpoint_file) as f:
        for line in f:
            line = line.strip()
            if line:
                done_topics.add(line)
    print(f"Loaded {len(done_topics)} checkpointed topics for {agent_name}")
except FileNotFoundError:
    pass
sys.stdout.flush()

# Fetch all adjudicated claims (axioms)
axioms = []
try:
    url = f"{server}/api/claims?status=adjudicated&limit=100"
    req = urllib.request.Request(url, headers={"User-Agent": "ground-seed/1.0"})
    resp = urllib.request.urlopen(req)
    data = json.loads(resp.read().decode())
    axioms = data.get("data", [])
    print(f"Fetched {len(axioms)} axioms")
except Exception as e:
    print(f"Error fetching axioms: {e}")
sys.stdout.flush()

axiom_text = "\n".join(
    f"- ID: {a['id']}\n  Proposition: {a['proposition']}"
    for a in axioms
)

topic_slugs = [s.strip() for s in agent_topics.split(",")]
posted = 0
errors = 0
dupes = 0

for slug in topic_slugs:
    if slug in done_topics:
        print(f"[{slug}] already checkpointed, skipping")
        sys.stdout.flush()
        continue

    # Fetch claims for this topic
    try:
        url = f"{server}/api/topics/{slug}/claims?limit=100"
        req = urllib.request.Request(url, headers={"User-Agent": "ground-seed/1.0"})
        resp = urllib.request.urlopen(req)
        data = json.loads(resp.read().decode())
        topic_claims = data.get("data", [])
    except Exception as e:
        print(f"[{slug}] fetch error: {e}")
        sys.stdout.flush()
        errors += 1
        continue

    # Filter out adjudicated claims (axioms can be targets but not sources)
    non_axiom = [c for c in topic_claims if c.get("status") != "adjudicated"]
    print(f"[{slug}] {len(non_axiom)} non-axiom claims, {len(topic_claims)} total")
    sys.stdout.flush()

    if not non_axiom:
        with open(checkpoint_file, "a") as f:
            f.write(slug + "\n")
        continue

    # Batch claims into groups of BATCH_SIZE
    batches = [non_axiom[i:i+BATCH_SIZE] for i in range(0, len(non_axiom), BATCH_SIZE)]

    for bi, batch in enumerate(batches):
        claims_text = "\n".join(
            f"- ID: {c['id']}\n  Proposition: {c['proposition']}\n  Status: {c.get('status', 'active')}"
            for c in batch
        )

        # Build prompt
        prompt_body = task_template.replace("{{CLAIMS}}", claims_text).replace("{{AXIOMS}}", axiom_text)
        full_prompt = personality + "\n\n---\n\n" + prompt_body

        print(f"[{slug}] batch {bi+1}/{len(batches)} ({len(batch)} claims)", end="", flush=True)

        # Call claude
        try:
            result = subprocess.run(
                ["claude", "--model", "sonnet", "-p", full_prompt],
                capture_output=True, text=True, timeout=180
            )
            response = result.stdout.strip()
        except subprocess.TimeoutExpired:
            print(" -> TIMEOUT")
            sys.stdout.flush()
            errors += 1
            continue
        except Exception as e:
            print(f" -> ERROR: {e}")
            sys.stdout.flush()
            errors += 1
            continue

        if not response:
            print(" -> empty response")
            sys.stdout.flush()
            errors += 1
            continue

        # Strip markdown fences
        text = response
        if text.startswith("```"):
            first_nl = text.index("\n")
            text = text[first_nl + 1:]
            if "```" in text:
                text = text[:text.rindex("```")]
            text = text.strip()

        # Parse JSON array
        try:
            edges = json.loads(text)
        except json.JSONDecodeError:
            match = re.search(r"\[[\s\S]*\]", text)
            if match:
                try:
                    edges = json.loads(match.group())
                except json.JSONDecodeError:
                    print(f" -> JSON parse error: {text[:100]}")
                    sys.stdout.flush()
                    errors += 1
                    continue
            else:
                print(f" -> no JSON array: {text[:100]}")
                sys.stdout.flush()
                errors += 1
                continue

        if not isinstance(edges, list):
            print(f" -> not a list: {type(edges)}")
            sys.stdout.flush()
            errors += 1
            continue

        batch_posted = 0
        batch_dupes = 0
        for edge in edges:
            claim_id = edge.get("claim_id", "")
            depends_on_id = edge.get("depends_on_id", "")
            strength = edge.get("strength", 0.5)
            reasoning = edge.get("reasoning", "")

            if not claim_id or not depends_on_id:
                continue
            if claim_id == depends_on_id:
                continue

            body = {
                "claim_id": claim_id,
                "depends_on_id": depends_on_id,
                "strength": strength,
                "reasoning": reasoning,
            }

            req = urllib.request.Request(
                f"{server}/api/dependencies",
                data=json.dumps(body).encode(),
                headers={
                    "Content-Type": "application/json",
                    "Authorization": f"Bearer {token}",
                    "User-Agent": "ground-seed/1.0",
                },
            )
            try:
                resp = urllib.request.urlopen(req)
                resp_data = json.loads(resp.read().decode())
                # 201 = new, 200 = already existed
                if resp.status == 201:
                    batch_posted += 1
                else:
                    batch_dupes += 1
            except urllib.error.HTTPError as e:
                err = e.read().decode()[:200]
                if e.code == 400 and "CYCLE" in err:
                    print(f" [cycle skip]", end="", flush=True)
                else:
                    print(f" [POST {e.code}]", end="", flush=True)
                    errors += 1
            except Exception as e:
                print(f" [err: {e}]", end="", flush=True)
                errors += 1

        posted += batch_posted
        dupes += batch_dupes
        print(f" -> {batch_posted} new, {batch_dupes} dupes")
        sys.stdout.flush()

    # Checkpoint topic
    with open(checkpoint_file, "a") as f:
        f.write(slug + "\n")

print(f"\nDone: {posted} dependencies posted, {dupes} dupes, {errors} errors")
PYEOF
            exit_code=$?

            tail_line=$(tail -1 "$log" 2>/dev/null || echo "")
            if echo "$tail_line" | grep -q "0 errors"; then
                touch "$WORK_DIR/.checkpoints/dep-${agent_name}.done"
            fi

            echo "[dep] $agent_name complete (exit $exit_code)"
        ) >> "/tmp/ground-agents-dep-${agent}.log" 2>&1 &
        pids+=($!)
        running=$((running + 1))

        if [ "$running" -ge "$CONCURRENCY" ]; then
            wait -n "${pids[@]}" 2>/dev/null || true
            running=$((running - 1))
        fi
    done

    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    echo "[dep] All agents complete"
}

# --- Round 2: Per-claim adversarial evaluation ---
run_round_2() {
    echo ""
    echo "=========================================="
    echo "  ROUND 2: Adversarial Evaluation"
    echo "=========================================="

    local pids=()
    local running=0

    for agent in "${AGENTS[@]}"; do
        if [ -f "$WORK_DIR/.checkpoints/r2-${agent}.done" ]; then
            echo "[r2] $agent: already done, skipping"
            continue
        fi

        echo "[r2] starting $agent..."

        (
            agent_name="$agent"
            log="/tmp/ground-agents-r2-${agent_name}.log"
            token="${TOKENS[$agent_name]}"
            topics="${TOPICS[$agent_name]}"
            personality=$(cat "$REPO/prompts/${agent_name}.md")
            standard_template=$(cat "$REPO/tasks/seed-round-2-evaluate.md")
            standard_template="${standard_template//\{\{TOPICS\}\}/$topics}"
            challenge_template=$(cat "$REPO/tasks/seed-round-2-challenge.md")
            challenge_template="${challenge_template//\{\{TOPICS\}\}/$topics}"
            checkpoint_file="$WORK_DIR/.checkpoints/r2-${agent_name}-claims.txt"

            echo "$(date +%H:%M:%S) starting round 2 for $agent_name" > "$log"

            AGENT_TOPICS="$topics" PERSONALITY="$personality" \
            STANDARD_TEMPLATE="$standard_template" CHALLENGE_TEMPLATE="$challenge_template" \
            API_SERVER="$SERVER" API_TOKEN="$token" \
            AGENT_NAME="$agent_name" CHECKPOINT_FILE="$checkpoint_file" \
            python3 << 'PYEOF'
import os, json, re, subprocess, sys, urllib.request, urllib.error, hashlib, random

agent_topics = os.environ["AGENT_TOPICS"]
personality = os.environ["PERSONALITY"]
standard_template = os.environ["STANDARD_TEMPLATE"]
challenge_template = os.environ["CHALLENGE_TEMPLATE"]
server = os.environ["API_SERVER"]
token = os.environ["API_TOKEN"]
agent_name = os.environ["AGENT_NAME"]
checkpoint_file = os.environ["CHECKPOINT_FILE"]

# Contest quotas per personality
CHALLENGE_QUOTA = {
    "contrarian": 0.60, "skeptic": 0.50,
    "formalist": 0.35, "empiricist": 0.35, "reductionist": 0.35,
    "analyst": 0.30, "historian": 0.30,
    "bayesian": 0.25, "pragmatist": 0.25,
    "synthesizer": 0.20, "contextualist": 0.20, "phenomenologist": 0.20,
}

quota = CHALLENGE_QUOTA.get(agent_name, 0.25)

# Load checkpoint (set of already-processed claim IDs)
done_claims = set()
try:
    with open(checkpoint_file) as f:
        for line in f:
            line = line.strip()
            if line:
                done_claims.add(line)
    print(f"Loaded {len(done_claims)} checkpointed claims for {agent_name}")
except FileNotFoundError:
    pass
sys.stdout.flush()

# Fetch claims per topic, deduplicate
seen_ids = set()
matching = []
topic_slugs = [s.strip() for s in agent_topics.split(",")]
for slug in topic_slugs:
    try:
        url = f"{server}/api/topics/{slug}/claims?limit=100"
        req = urllib.request.Request(url, headers={"User-Agent": "ground-seed/1.0"})
        resp = urllib.request.urlopen(req)
        data = json.loads(resp.read().decode())
        topic_claims = data.get("data", [])
        for c in topic_claims:
            if c["id"] not in seen_ids:
                seen_ids.add(c["id"])
                matching.append(c)
        print(f"  {slug}: {len(topic_claims)} claims")
    except Exception as e:
        print(f"  {slug}: fetch error: {e}")
    sys.stdout.flush()

# Deterministic shuffle seeded by agent name (stable across restarts)
seed = int(hashlib.md5(agent_name.encode()).hexdigest()[:8], 16)
rng = random.Random(seed)
rng.shuffle(matching)

# Split: first N% get challenge prompt, rest get standard
n_challenge = int(len(matching) * quota)
challenge_ids = set(c["id"] for c in matching[:n_challenge])

print(f"Total {len(matching)} claims for {agent_name}: {n_challenge} challenge, {len(matching) - n_challenge} standard (quota={quota})")
sys.stdout.flush()

posted = 0
skipped = 0
contested = 0
errors = 0
checkpointed = 0

for i, claim in enumerate(matching):
    cid = claim["id"]

    if cid in done_claims:
        checkpointed += 1
        continue

    is_challenge = cid in challenge_ids
    prop = claim["proposition"]
    g = claim.get("groundedness", 0) or 0
    status = claim.get("status", "active")

    claim_text = f"ID: {cid}\nProposition: {prop}\nStatus: {status}\nGroundedness: {g:.2f}"

    if is_challenge:
        prompt_body = challenge_template.replace("{{CLAIM}}", claim_text)
    else:
        prompt_body = standard_template.replace("{{CLAIM}}", claim_text)

    full_prompt = personality + "\n\n---\n\n" + prompt_body

    mode = "CHAL" if is_challenge else "std"
    print(f"[{i+1}/{len(matching)}] [{mode}] {cid[:8]}... {prop[:50]}", end="", flush=True)

    # Call claude
    try:
        result = subprocess.run(
            ["claude", "--model", "sonnet", "-p", full_prompt],
            capture_output=True, text=True, timeout=120
        )
        response = result.stdout.strip()
    except subprocess.TimeoutExpired:
        print(" -> TIMEOUT")
        sys.stdout.flush()
        errors += 1
        continue
    except Exception as e:
        print(f" -> ERROR: {e}")
        sys.stdout.flush()
        errors += 1
        continue

    if not response:
        print(" -> empty response")
        sys.stdout.flush()
        errors += 1
        continue

    # Strip markdown fences
    text = response
    if text.startswith("```"):
        first_nl = text.index("\n")
        text = text[first_nl + 1:]
        if "```" in text:
            text = text[:text.rindex("```")]
        text = text.strip()

    # Parse JSON
    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        match = re.search(r"\{[\s\S]*\}", text)
        if match:
            try:
                data = json.loads(match.group())
            except json.JSONDecodeError:
                print(f" -> JSON parse error: {text[:100]}")
                sys.stdout.flush()
                errors += 1
                continue
        else:
            print(f" -> no JSON: {text[:100]}")
            sys.stdout.flush()
            errors += 1
            continue

    # Challenge mode: skip not allowed, retry not worth it — just don't checkpoint
    if data.get("action") == "skip":
        if is_challenge:
            print(" -> skip on challenge (not checkpointed, will retry)")
            sys.stdout.flush()
            errors += 1
            continue
        print(" -> skip")
        sys.stdout.flush()
        skipped += 1
        with open(checkpoint_file, "a") as f:
            f.write(cid + "\n")
        continue

    # Convert refine -> contest
    stance = data.get("stance", "support")
    if stance == "refine":
        stance = "contest"
        print(" [refine->contest]", end="", flush=True)

    # Challenge mode: force contest if agent returned support
    if is_challenge and stance != "contest":
        stance = "contest"
        print(" [forced->contest]", end="", flush=True)

    if stance == "contest":
        contested += 1

    body = {
        "claim_id": cid,
        "stance": stance,
        "confidence": data.get("confidence", 0.5),
        "reasoning": data.get("reasoning", ""),
    }

    req = urllib.request.Request(
        f"{server}/api/assertions",
        data=json.dumps(body).encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
            "User-Agent": "ground-seed/1.0",
        },
    )
    try:
        resp = urllib.request.urlopen(req)
        resp.read()
        posted += 1
        print(f" -> {stance} c={data.get('confidence', 0.5)}")
        with open(checkpoint_file, "a") as f:
            f.write(cid + "\n")
    except urllib.error.HTTPError as e:
        err = e.read().decode()[:200]
        print(f" -> POST {e.code}: {err}")
        errors += 1
    except Exception as e:
        print(f" -> POST error: {e}")
        errors += 1
    sys.stdout.flush()

if checkpointed > 0:
    print(f"Resumed: skipped {checkpointed} already-processed claims")
contest_rate = (contested / posted * 100) if posted > 0 else 0
print(f"\nDone: {posted} posted ({contested} contests, {contest_rate:.0f}%), {skipped} skipped, {errors} errors")
PYEOF
            exit_code=$?

            tail_line=$(tail -1 "$log" 2>/dev/null || echo "")
            if echo "$tail_line" | grep -q "0 errors"; then
                touch "$WORK_DIR/.checkpoints/r2-${agent_name}.done"
            fi

            echo "[r2] $agent_name complete (exit $exit_code)"
        ) >> "/tmp/ground-agents-r2-${agent}.log" 2>&1 &
        pids+=($!)
        running=$((running + 1))

        if [ "$running" -ge "$CONCURRENCY" ]; then
            wait -n "${pids[@]}" 2>/dev/null || true
            running=$((running - 1))
        fi
    done

    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    echo "[r2] All agents complete"
}

# --- Round 3: Per-assertion review ---
run_round_3() {
    echo ""
    echo "=========================================="
    echo "  ROUND 3: Per-Assertion Review"
    echo "=========================================="

    # Fetch all claims
    echo "[r3] Fetching claims..."
    local claims_file="$WORK_DIR/claims.json"
    curl -sf "$SERVER/api/claims?limit=500" > "$claims_file"

    local claim_ids
    claim_ids=$(python3 -c "
import json
for c in json.load(open('$claims_file')).get('data', []):
    print(c['id'])
")

    # Fetch claim details (includes assertions) in parallel
    local details_dir="$WORK_DIR/claim-details"
    mkdir -p "$details_dir"

    echo "[r3] Fetching claim details..."
    local fetch_pids=()
    local fetch_running=0
    local fetch_total=0
    while IFS= read -r cid; do
        curl -sf "$SERVER/api/claims/$cid" > "$details_dir/$cid.json" &
        fetch_pids+=($!)
        fetch_running=$((fetch_running + 1))
        fetch_total=$((fetch_total + 1))
        if [ "$fetch_running" -ge 20 ]; then
            wait -n "${fetch_pids[@]}" 2>/dev/null || true
            fetch_running=$((fetch_running - 1))
        fi
    done <<< "$claim_ids"
    for pid in "${fetch_pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done
    echo "[r3] Fetched details for $fetch_total claims"

    local pids=()
    local running=0

    for agent in "${AGENTS[@]}"; do
        if [ -f "$WORK_DIR/.checkpoints/r3-${agent}.done" ]; then
            echo "[r3] $agent: already done, skipping"
            continue
        fi

        echo "[r3] starting $agent..."

        (
            agent_name="$agent"
            agent_id="seed-${agent_name}"
            log="/tmp/ground-agents-r3-${agent_name}.log"
            token="${TOKENS[$agent_name]}"
            topics="${TOPICS[$agent_name]}"
            personality=$(cat "$REPO/prompts/${agent_name}.md")
            task_template=$(cat "$REPO/tasks/seed-round-3-review.md")
            checkpoint_file="$WORK_DIR/.checkpoints/r3-${agent_name}-assertions.txt"

            echo "$(date +%H:%M:%S) starting round 3 for $agent_name" > "$log"

            # Python: iterate claim details, find other agents' assertions, review one-by-one
            DETAILS_DIR="$details_dir" AGENT_ID="$agent_id" PERSONALITY="$personality" \
            TASK_TEMPLATE="$task_template" API_SERVER="$SERVER" API_TOKEN="$token" \
            AGENT_NAME="$agent_name" CHECKPOINT_FILE="$checkpoint_file" \
            python3 << 'PYEOF'
import os, json, re, glob, subprocess, sys, urllib.request, urllib.error

details_dir = os.environ["DETAILS_DIR"]
agent_id = os.environ["AGENT_ID"]
personality = os.environ["PERSONALITY"]
task_template = os.environ["TASK_TEMPLATE"]
server = os.environ["API_SERVER"]
token = os.environ["API_TOKEN"]
agent_name = os.environ["AGENT_NAME"]
checkpoint_file = os.environ["CHECKPOINT_FILE"]

# Load checkpoint (set of already-reviewed assertion IDs)
done_assertions = set()
try:
    with open(checkpoint_file) as f:
        for line in f:
            line = line.strip()
            if line:
                done_assertions.add(line)
    print(f"Loaded {len(done_assertions)} checkpointed assertions for {agent_name}")
except FileNotFoundError:
    pass
sys.stdout.flush()

# Collect all assertions by other agents
to_review = []  # list of (claim_info, assertion)
for fpath in sorted(glob.glob(f"{details_dir}/*.json")):
    try:
        with open(fpath) as f:
            detail = json.load(f)
        data = detail.get("data", {})
        claim = data.get("claim", {})
        assertions = data.get("assertions", [])

        for a in assertions:
            if a.get("agent_id") != agent_id:
                to_review.append((claim, a))
    except (json.JSONDecodeError, KeyError):
        continue

print(f"Found {len(to_review)} assertions to review for {agent_name}")
sys.stdout.flush()

posted = 0
errors = 0
checkpointed = 0

for i, (claim, assertion) in enumerate(to_review):
    aid = assertion["id"]

    # Skip if already checkpointed
    if aid in done_assertions:
        checkpointed += 1
        continue

    cid = claim["id"]
    prop = claim["proposition"]
    g = claim.get("groundedness", 0) or 0

    a_agent = assertion.get("agent_id", "unknown")
    stance = assertion["stance"]
    conf = assertion["confidence"]
    reasoning = (assertion.get("reasoning") or "")[:600]

    # Build claim text
    claim_text = f"ID: {cid}\nProposition: {prop}\nGroundedness: {g:.2f}"

    # Build assertion text
    assertion_text = f"ID: {aid}\nBy: {a_agent}\nStance: {stance}\nConfidence: {conf}\nReasoning: {reasoning}"

    # Substitute into task template
    prompt_body = task_template.replace("{{CLAIM}}", claim_text).replace("{{ASSERTION}}", assertion_text)

    # Full prompt
    full_prompt = personality + "\n\n---\n\n" + prompt_body

    print(f"[{i+1}/{len(to_review)}] {aid[:8]}... by {a_agent}", end="", flush=True)

    # Call claude
    try:
        result = subprocess.run(
            ["claude", "--model", "sonnet", "-p", full_prompt],
            capture_output=True, text=True, timeout=120
        )
        response = result.stdout.strip()
    except subprocess.TimeoutExpired:
        print(" -> TIMEOUT")
        sys.stdout.flush()
        errors += 1
        continue
    except Exception as e:
        print(f" -> ERROR: {e}")
        sys.stdout.flush()
        errors += 1
        continue

    if not response:
        print(" -> empty response")
        sys.stdout.flush()
        errors += 1
        continue

    # Strip markdown fences
    text = response
    if text.startswith("```"):
        first_nl = text.index("\n")
        text = text[first_nl + 1:]
        if "```" in text:
            text = text[:text.rindex("```")]
        text = text.strip()

    # Parse JSON
    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        match = re.search(r"\{[\s\S]*\}", text)
        if match:
            try:
                data = json.loads(match.group())
            except json.JSONDecodeError:
                print(f" -> JSON parse error: {text[:100]}")
                sys.stdout.flush()
                errors += 1
                continue
        else:
            print(f" -> no JSON: {text[:100]}")
            sys.stdout.flush()
            errors += 1
            continue

    # POST review
    body = {
        "assertion_id": aid,
        "helpfulness": data["helpfulness"],
        "reasoning": data.get("reasoning", ""),
    }

    req = urllib.request.Request(
        f"{server}/api/reviews",
        data=json.dumps(body).encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
            "User-Agent": "ground-seed/1.0",
        },
    )
    try:
        resp = urllib.request.urlopen(req)
        resp.read()
        posted += 1
        print(f" -> h={data['helpfulness']}")
        # Checkpoint successful post
        with open(checkpoint_file, "a") as f:
            f.write(aid + "\n")
    except urllib.error.HTTPError as e:
        err = e.read().decode()[:200]
        print(f" -> POST {e.code}: {err}")
        errors += 1
    except Exception as e:
        print(f" -> POST error: {e}")
        errors += 1
    sys.stdout.flush()

if checkpointed > 0:
    print(f"Resumed: skipped {checkpointed} already-reviewed assertions")
print(f"\nDone: {posted} posted, {errors} errors")
PYEOF
            exit_code=$?

            tail_line=$(tail -1 "$log" 2>/dev/null || echo "")
            if echo "$tail_line" | grep -q "0 errors"; then
                touch "$WORK_DIR/.checkpoints/r3-${agent_name}.done"
            fi

            echo "[r3] $agent_name complete (exit $exit_code)"
        ) >> "/tmp/ground-agents-r3-${agent}.log" 2>&1 &
        pids+=($!)
        running=$((running + 1))

        if [ "$running" -ge "$CONCURRENCY" ]; then
            wait -n "${pids[@]}" 2>/dev/null || true
            running=$((running - 1))
        fi
    done

    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    echo "[r3] All agents complete"
}

# --- Run selected rounds ---
if [ "$ROUND" = "all" ] || [ "$ROUND" = "dep" ]; then
    run_round_dep
fi

if [ "$ROUND" = "all" ] || [ "$ROUND" = "2" ]; then
    run_round_2
fi

if [ "$ROUND" = "all" ] || [ "$ROUND" = "3" ]; then
    run_round_3
fi

# --- Compute epoch on server ---
echo ""
echo "=== Computing epoch on server ==="
ssh "$HOST" "export \$(cat /root/.ground/env | xargs) && /opt/ground-bin compute --db $REMOTE_DB"

echo ""
echo "=== Done ==="
echo "View results: $SERVER"
