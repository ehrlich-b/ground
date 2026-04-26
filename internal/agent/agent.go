// Package agent provides helpers for registering Ground agents and seeding axioms.
//
// v2 dropped the 12-personality-EigenTrust seed orchestration. Phase 12 will rebuild
// search/extract/audit seed rounds against the v2 API. Until then, this package
// only exposes the lower-level helpers used by `ground bootstrap-axioms` and
// `ground seed-agent`.
package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/ehrlich-b/ground/internal/api"
	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

// Axiom represents a parsed axiom from FACTS.md.
type Axiom struct {
	Code        string
	Proposition string
	Basis       string
	Anchors     []string
	Value       float64 // 1.0 = true, 0.0 = false
	Citations   []AxiomCitation
}

// AxiomCitation is a verbatim-quote citation backing an axiom.
type AxiomCitation struct {
	URL           string
	VerbatimQuote string
	Polarity      string
	Reasoning     string
}

// ParseAxioms parses FACTS.md content into structured axioms.
//
// Format expected:
//
//	### MATH-01: <title>
//	**Proposition**: ...
//	**Basis**: ...
//	**Anchors**: tag-a, tag-b
//	**Adjudicated**: TRUE | FALSE
//	**Citation**: <url> | <verbatim quote>
//	**Citation**: <url> | <verbatim quote>
//
// All axioms in `## Adjudicated FALSE` default to Value=0; `Adjudicated:` line can override.
func ParseAxioms(content string) []Axiom {
	var axioms []Axiom
	lines := strings.Split(content, "\n")
	var current *Axiom
	inFalse := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "## Adjudicated FALSE" {
			inFalse = true
			continue
		}
		if strings.HasPrefix(line, "### ") {
			code := strings.TrimPrefix(line, "### ")
			if idx := strings.Index(code, ":"); idx > 0 {
				code = strings.TrimSpace(code[:idx])
			}
			ax := Axiom{Code: code, Value: 1.0}
			if inFalse {
				ax.Value = 0.0
			}
			axioms = append(axioms, ax)
			current = &axioms[len(axioms)-1]
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "**Proposition**:"):
			current.Proposition = strings.TrimSpace(strings.TrimPrefix(line, "**Proposition**:"))
		case strings.HasPrefix(line, "**Basis**:"):
			current.Basis = strings.TrimSpace(strings.TrimPrefix(line, "**Basis**:"))
		case strings.HasPrefix(line, "**Anchors**:"):
			body := strings.TrimSpace(strings.TrimPrefix(line, "**Anchors**:"))
			for _, a := range strings.Split(body, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					current.Anchors = append(current.Anchors, a)
				}
			}
		case strings.HasPrefix(line, "**Adjudicated**:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "**Adjudicated**:"))
			if strings.EqualFold(val, "FALSE") {
				current.Value = 0.0
			}
		case strings.HasPrefix(line, "**Citation**:"):
			body := strings.TrimSpace(strings.TrimPrefix(line, "**Citation**:"))
			parts := strings.SplitN(body, "|", 2)
			if len(parts) != 2 {
				continue
			}
			cit := AxiomCitation{
				URL:           strings.TrimSpace(parts[0]),
				VerbatimQuote: strings.TrimSpace(parts[1]),
				Polarity:      "supports",
			}
			if current.Value == 0.0 {
				cit.Polarity = "contradicts"
			}
			current.Citations = append(current.Citations, cit)
		}
	}
	return axioms
}

// SeedAxioms creates and adjudicates each axiom as a claim. Citations on each axiom
// are persisted via Phase 3 once the citation pipeline is wired into the seed flow;
// for now this seeds the bare claims so the rest of the system can operate.
func SeedAxioms(store *db.Store, axioms []Axiom) error {
	for _, ax := range axioms {
		id := fmt.Sprintf("axiom-%s", strings.ToLower(ax.Code))
		if _, err := store.GetClaim(id); err == nil {
			log.Printf("  axiom %s already exists, skipping", ax.Code)
			continue
		}
		claim := &model.Claim{
			ID:          id,
			Proposition: ax.Proposition,
			Status:      "active",
		}
		if err := store.CreateClaim(claim); err != nil {
			return fmt.Errorf("create axiom %s: %w", ax.Code, err)
		}
		reasoning := fmt.Sprintf("Axiomatic node. %s", ax.Basis)
		if err := store.AdjudicateClaim(id, ax.Value, "seed", reasoning); err != nil {
			return fmt.Errorf("adjudicate axiom %s: %w", ax.Code, err)
		}
		status := "TRUE"
		if ax.Value == 0.0 {
			status = "FALSE"
		}
		log.Printf("  %s [%s]: %s", ax.Code, status, truncate(ax.Proposition, 70))
	}
	return nil
}

// EnsureAdminAgent creates the admin agent (idempotent) and returns it.
func EnsureAdminAgent(store *db.Store) (*model.Agent, error) {
	if existing, err := store.GetAgent("admin"); err == nil {
		return existing, nil
	}
	a := &model.Agent{
		ID:           "admin",
		Name:         "admin",
		Role:         "admin",
		Reliability:  1.0,
		Productivity: 0.0,
	}
	if err := store.CreateAgent(a); err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	return a, nil
}

// IssueTokenFor a registered agent, hooking through the api package's helpers.
func IssueTokenFor(store *db.Store, jwtSecret []byte, agentID, role string) (string, error) {
	return api.IssueToken(store, jwtSecret, agentID, role)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
