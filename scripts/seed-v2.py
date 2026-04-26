#!/usr/bin/env python3
"""
Ground v2 seed orchestrator.

Drives the v2 round order — search → extract → audit → deps → compute — against a
running Ground server. Replaces the v1 bash harness (run-seed-agents.sh), which
still exists for archival reference but targets dead endpoints (/api/assertions,
/api/reviews) and v1 contest-quota logic.

This script assumes:
  - `ground serve` is running and reachable at $SERVER (default http://localhost:8080)
  - `claude` CLI is installed and authenticated
  - Personality prompts live at $REPO/prompts/<name>.md (search/extract bias only —
    NOT epistemic stances; v2 does not force contest quotas)
  - Task templates live at $REPO/tasks/v2-{search,extract,audit,deps}.md
  - Each seed agent has been registered server-side; this script mints fresh tokens
    for them via `ground token --agent-id <id>` against the server's local DB.

Usage:
    python3 scripts/seed-v2.py [round] [options]

    round: search | extract | audit | deps | compute | all

Options:
    --server URL       (default: $GROUND_URL or http://localhost:8080)
    --db PATH          (default: $GROUND_DB or ground.db) — used for token minting
                       and `ground compute`. Server must be using this DB.
    --bin PATH         (default: ./ground) — ground binary for offline ops
    --agents NAMES     comma-separated personality names (default: all 12)
    --topics SLUGS     restrict to these topic slugs (default: all)
    --limit N          per-round work cap (default: 50 per agent)
    --dry-run          print prompts and intended POSTs without calling claude/API
    --model NAME       claude model to use (default: sonnet)
"""

from __future__ import annotations
import argparse
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Iterable

# --- Defaults ---

REPO = pathlib.Path(__file__).resolve().parent.parent
DEFAULT_SERVER = os.environ.get("GROUND_URL", "http://localhost:8080")
DEFAULT_DB = os.environ.get("GROUND_DB", "ground.db")
DEFAULT_BIN = str(REPO / "ground")

# Personality bias — search/extraction style only. Each gets ~3 topic slugs.
# Empty for now; populate from TOPICS.md as the topic taxonomy stabilises.
PERSONALITIES = [
    "empiricist", "formalist", "historian", "skeptic",
    "synthesizer", "pragmatist", "contrarian", "analyst",
    "contextualist", "bayesian", "phenomenologist", "reductionist",
]


# --- HTTP helpers ---

def http_request(method: str, url: str, *, token: str | None = None, body: Any = None) -> tuple[int, Any]:
    headers = {"User-Agent": "ground-seed-v2/1.0"}
    data = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(body).encode()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            payload = resp.read().decode()
            return resp.status, json.loads(payload) if payload else None
    except urllib.error.HTTPError as e:
        try:
            err = json.loads(e.read().decode())
        except Exception:
            err = {"error": {"code": "UNKNOWN", "message": str(e)}}
        return e.code, err


def get_data(url: str, token: str | None = None) -> list | dict:
    status, payload = http_request("GET", url, token=token)
    if status >= 400:
        raise RuntimeError(f"GET {url} -> {status}: {payload}")
    return payload.get("data", payload) if isinstance(payload, dict) else payload


# --- Token minting (offline, uses CLI against local DB) ---

def mint_token(bin_path: str, db_path: str, agent_id: str) -> str:
    res = subprocess.run(
        [bin_path, "token", "--agent-id", agent_id, "--db", db_path],
        capture_output=True, text=True, check=False,
    )
    if res.returncode != 0:
        raise RuntimeError(f"token mint for {agent_id} failed: {res.stderr.strip()}")
    return res.stdout.strip()


# --- Claude wrapper ---

def call_claude(prompt: str, *, model: str = "sonnet", timeout: int = 180, dry_run: bool = False) -> str:
    if dry_run:
        print("    [dry-run] would call claude with prompt:")
        for line in prompt.splitlines()[:10]:
            print(f"      {line}")
        if len(prompt.splitlines()) > 10:
            print(f"      ... ({len(prompt.splitlines()) - 10} more lines)")
        return "[]"
    res = subprocess.run(
        ["claude", "--model", model, "-p", prompt],
        capture_output=True, text=True, timeout=timeout, check=False,
    )
    return res.stdout.strip()


def parse_json(text: str) -> Any:
    """Parse a JSON value from a claude response, tolerating markdown fences."""
    if text.startswith("```"):
        first_nl = text.find("\n")
        if first_nl > 0:
            text = text[first_nl + 1:]
        if "```" in text:
            text = text[: text.rindex("```")]
        text = text.strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        # Salvage: take the largest [...] or {...} substring
        for opener, closer in [("[", "]"), ("{", "}")]:
            i = text.find(opener)
            j = text.rfind(closer)
            if 0 <= i < j:
                try:
                    return json.loads(text[i : j + 1])
                except json.JSONDecodeError:
                    continue
        raise


# --- Round implementations ---

def load_template(name: str) -> str:
    return (REPO / "tasks" / f"v2-{name}.md").read_text()


def load_personality(name: str) -> str:
    p = REPO / "prompts" / f"{name}.md"
    return p.read_text() if p.exists() else ""


def round_search(args, tokens: dict[str, str]) -> None:
    """For each (agent, topic), propose candidate sources via /api/sources/candidates."""
    template = load_template("search")
    topics = args.topics or [t["slug"] for t in get_data(f"{args.server}/api/topics")]

    for agent in args.agents:
        token = tokens.get(agent)
        if not token:
            print(f"[search] {agent}: no token, skipping")
            continue
        personality = load_personality(agent)
        for slug in topics:
            print(f"[search] {agent} -> {slug}")
            try:
                topic = get_data(f"{args.server}/api/topics/{slug}")
                claims = get_data(f"{args.server}/api/topics/{slug}/claims?limit=50") or []
            except Exception as e:
                print(f"  fetch error: {e}")
                continue
            if not claims:
                continue
            claims_text = "\n".join(
                f"- {c['id']}: {c['proposition']}" for c in claims[: args.limit]
            )
            prompt = template
            prompt = prompt.replace("{{TOPIC_SLUG}}", slug)
            prompt = prompt.replace("{{TOPIC_TITLE}}", topic.get("title", slug))
            prompt = prompt.replace("{{CLAIMS}}", claims_text)
            full = (personality + "\n\n---\n\n" + prompt) if personality else prompt
            response = call_claude(full, model=args.model, dry_run=args.dry_run)
            if args.dry_run:
                continue
            try:
                proposals = parse_json(response)
            except Exception as e:
                print(f"  parse error: {e}; response: {response[:200]}")
                continue
            if not isinstance(proposals, list):
                print(f"  expected list, got {type(proposals).__name__}")
                continue
            candidates = [
                {"url": p.get("url", "").strip(), "reasoning": p.get("reasoning", "")}
                for p in proposals if p.get("url")
            ]
            if not candidates:
                continue
            status, payload = http_request(
                "POST", f"{args.server}/api/sources/candidates",
                token=token,
                body={"candidates": candidates, "topic_slug": slug},
            )
            ingested = payload.get("data", []) if isinstance(payload, dict) else []
            ok = sum(1 for r in ingested if "error" not in r)
            print(f"  {ok}/{len(candidates)} ingested (status {status})")


def round_extract(args, tokens: dict[str, str]) -> None:
    """For each claim with a candidate source, propose citations and POST them."""
    template = load_template("extract")
    topics = args.topics or [t["slug"] for t in get_data(f"{args.server}/api/topics")]

    for agent in args.agents:
        token = tokens.get(agent)
        if not token:
            continue
        personality = load_personality(agent)
        seen_pairs: set[tuple[str, str]] = set()
        posted = 0

        for slug in topics:
            try:
                claims = get_data(f"{args.server}/api/topics/{slug}/claims?limit=50") or []
            except Exception:
                continue
            for claim in claims:
                if posted >= args.limit:
                    break
                cid = claim["id"]
                if claim.get("status") == "adjudicated":
                    continue
                # Find recently ingested sources for this topic. Without a topic→source
                # join, we use the global recent-sources list as a proxy.
                sources = get_data(f"{args.server}/api/sources?limit=20") or []
                for src in sources:
                    pair = (cid, src["id"])
                    if pair in seen_pairs:
                        continue
                    seen_pairs.add(pair)
                    body_url = f"{args.server}/api/sources/{src['id']}/body"
                    try:
                        with urllib.request.urlopen(body_url, timeout=30) as resp:
                            body_text = resp.read().decode("utf-8", errors="replace")
                    except Exception:
                        continue
                    if not body_text or len(body_text) < 200:
                        continue
                    body_excerpt = body_text[:6000]
                    prompt = template
                    prompt = prompt.replace("{{CLAIM_ID}}", cid)
                    prompt = prompt.replace("{{CLAIM_PROPOSITION}}", claim["proposition"])
                    prompt = prompt.replace("{{SOURCE_ID}}", src["id"])
                    prompt = prompt.replace("{{SOURCE_URL}}", src["url"])
                    prompt = prompt.replace("{{SOURCE_TITLE}}", src.get("title") or src["url"])
                    prompt = prompt.replace("{{SOURCE_BODY}}", body_excerpt)
                    full = (personality + "\n\n---\n\n" + prompt) if personality else prompt
                    response = call_claude(full, model=args.model, dry_run=args.dry_run)
                    if args.dry_run:
                        continue
                    try:
                        items = parse_json(response)
                    except Exception:
                        continue
                    if not isinstance(items, list):
                        continue
                    for item in items:
                        body = {
                            "claim_id": cid,
                            "source_id": src["id"],
                            "verbatim_quote": item.get("verbatim_quote", ""),
                            "polarity": item.get("polarity", "supports"),
                            "reasoning": item.get("reasoning", ""),
                        }
                        if not body["verbatim_quote"]:
                            continue
                        status, payload = http_request(
                            "POST", f"{args.server}/api/citations",
                            token=token, body=body,
                        )
                        if status == 201:
                            posted += 1
                            print(f"[extract] {agent} {cid[:8]}/{src['id'][:8]}: cite #{posted}")
                        elif status == 400:
                            # Mechanical fail or duplicate — expected, don't spam logs
                            pass
                        else:
                            print(f"  POST citation -> {status}: {payload}")
                    if posted >= args.limit:
                        break


def round_audit(args, tokens: dict[str, str]) -> None:
    """Pull the audit queue per agent and POST audits for each citation."""
    template = load_template("audit")

    for agent in args.agents:
        token = tokens.get(agent)
        if not token:
            continue
        personality = load_personality(agent)
        agent_id = f"seed-{agent}"
        try:
            queue = get_data(
                f"{args.server}/api/audits/queue?limit={args.limit}",
                token=token,
            ) or []
        except Exception as e:
            print(f"[audit] {agent}: fetch queue failed: {e}")
            continue
        print(f"[audit] {agent}: {len(queue)} citations to audit")
        for cit in queue:
            cid = cit["id"]
            src_id = cit["source_id"]
            try:
                src = get_data(f"{args.server}/api/sources/{src_id}")
                claim = get_data(f"{args.server}/api/claims/{cit['claim_id']}")
                body_url = f"{args.server}/api/sources/{src_id}/body"
                with urllib.request.urlopen(body_url, timeout=30) as resp:
                    body_text = resp.read().decode("utf-8", errors="replace")[:6000]
            except Exception:
                continue
            prompt = template
            prompt = prompt.replace("{{CITATION_ID}}", cid)
            prompt = prompt.replace("{{CLAIM_ID}}", cit["claim_id"])
            prompt = prompt.replace("{{CLAIM_PROPOSITION}}", claim.get("proposition", ""))
            prompt = prompt.replace("{{VERBATIM_QUOTE}}", cit["verbatim_quote"])
            prompt = prompt.replace("{{POLARITY}}", cit["polarity"])
            prompt = prompt.replace("{{EXTRACTOR_REASONING}}", cit.get("reasoning") or "")
            prompt = prompt.replace("{{SOURCE_ID}}", src_id)
            prompt = prompt.replace("{{SOURCE_URL}}", src.get("url", ""))
            prompt = prompt.replace("{{SOURCE_TITLE}}", src.get("title") or src.get("url", ""))
            prompt = prompt.replace("{{SOURCE_BODY}}", body_text)
            full = (personality + "\n\n---\n\n" + prompt) if personality else prompt
            response = call_claude(full, model=args.model, dry_run=args.dry_run)
            if args.dry_run:
                continue
            try:
                verdict = parse_json(response)
            except Exception:
                continue
            if not isinstance(verdict, dict):
                continue
            body = {
                "citation_id": cid,
                "semantic": verdict.get("semantic", "weak"),
                "reasoning": verdict.get("reasoning", ""),
            }
            status, payload = http_request(
                "POST", f"{args.server}/api/audits", token=token, body=body,
            )
            print(f"  {cid[:8]}: {body['semantic']} (status {status})")


def round_deps(args, tokens: dict[str, str]) -> None:
    template = load_template("deps")
    topics = args.topics or [t["slug"] for t in get_data(f"{args.server}/api/topics")]
    axioms = get_data(f"{args.server}/api/claims?status=adjudicated&limit=200") or []
    axioms_text = "\n".join(f"- {a['id']}: {a['proposition']}" for a in axioms)

    for agent in args.agents:
        token = tokens.get(agent)
        if not token:
            continue
        personality = load_personality(agent)
        for slug in topics:
            claims = get_data(f"{args.server}/api/topics/{slug}/claims?limit=50") or []
            non_axiom = [c for c in claims if c.get("status") != "adjudicated"]
            if not non_axiom:
                continue
            claims_text = "\n".join(f"- {c['id']}: {c['proposition']}" for c in non_axiom)
            prompt = template
            prompt = prompt.replace("{{CLAIMS}}", claims_text)
            prompt = prompt.replace("{{AXIOMS}}", axioms_text)
            full = (personality + "\n\n---\n\n" + prompt) if personality else prompt
            response = call_claude(full, model=args.model, dry_run=args.dry_run)
            if args.dry_run:
                continue
            try:
                edges = parse_json(response)
            except Exception:
                continue
            if not isinstance(edges, list):
                continue
            new = 0
            for e in edges:
                cid = e.get("claim_id")
                dep = e.get("depends_on_id")
                if not cid or not dep or cid == dep:
                    continue
                body = {
                    "claim_id": cid, "depends_on_id": dep,
                    "strength": e.get("strength", 0.5),
                    "reasoning": e.get("reasoning", ""),
                }
                status, _ = http_request(
                    "POST", f"{args.server}/api/dependencies", token=token, body=body,
                )
                if status == 201:
                    new += 1
            print(f"[deps] {agent} {slug}: {new} new edges")


def round_compute(args, _tokens: dict[str, str]) -> None:
    print("[compute] running epoch")
    res = subprocess.run(
        [args.bin, "compute", "--db", args.db],
        capture_output=True, text=True, check=False,
    )
    print(res.stdout)
    if res.returncode != 0:
        print(res.stderr, file=sys.stderr)


# --- Main ---

ROUNDS = {
    "search": round_search,
    "extract": round_extract,
    "audit": round_audit,
    "deps": round_deps,
    "compute": round_compute,
}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("round", choices=list(ROUNDS.keys()) + ["all"])
    parser.add_argument("--server", default=DEFAULT_SERVER)
    parser.add_argument("--db", default=DEFAULT_DB)
    parser.add_argument("--bin", default=DEFAULT_BIN)
    parser.add_argument("--agents", default=",".join(PERSONALITIES))
    parser.add_argument("--topics", default="")
    parser.add_argument("--limit", type=int, default=50)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--model", default="sonnet")
    args = parser.parse_args()

    args.agents = [a.strip() for a in args.agents.split(",") if a.strip()]
    args.topics = [t.strip() for t in args.topics.split(",") if t.strip()] if args.topics else []

    # Mint tokens (offline) — compute round skips this since it doesn't need API auth
    tokens: dict[str, str] = {}
    if args.round != "compute" and not args.dry_run:
        for agent in args.agents:
            try:
                tokens[agent] = mint_token(args.bin, args.db, f"seed-{agent}")
                print(f"[token] seed-{agent}: {tokens[agent][:16]}...")
            except Exception as e:
                print(f"[token] seed-{agent}: {e}")

    rounds = list(ROUNDS.keys()) if args.round == "all" else [args.round]
    for r in rounds:
        print(f"\n=== ROUND: {r} ===")
        ROUNDS[r](args, tokens)
    return 0


if __name__ == "__main__":
    sys.exit(main())
