package agent

import (
	"fmt"
	"os"
	"testing"
)

func TestParseAxioms(t *testing.T) {
	content := `# Ground — Axiomatic Nodes

## Mathematics

### MATH-01: Godel's First Incompleteness Theorem

**Proposition**: Any consistent formal system capable of expressing basic arithmetic contains statements that are true but unprovable within the system.

**Basis**: Proven (Godel, 1931). No serious dispute.

**Anchors**: godel-incompleteness-implications, p-vs-np, mathematics-discovered-or-invented, simulation-argument

### MATH-02: Godel's Second Incompleteness Theorem

**Proposition**: No consistent formal system capable of expressing basic arithmetic can prove its own consistency.

**Basis**: Proven (Godel, 1931).

**Anchors**: godel-incompleteness-implications, mathematics-discovered-or-invented

---

## Adjudicated FALSE

### FALSE-01: Local Hidden Variables

**Proposition**: Local hidden variable theories can account for all predictions of quantum mechanics.

**Basis**: Ruled out by Bell inequality violations (PHYS-01). Nobel Prize 2022.

**Adjudicated**: FALSE

**Anchors**: quantum-entanglement-and-locality
`

	axioms := ParseAxioms(content)

	if len(axioms) != 3 {
		t.Fatalf("expected 3 axioms, got %d", len(axioms))
	}

	// MATH-01
	if axioms[0].Code != "MATH-01" {
		t.Errorf("axiom 0 code: got %q, want MATH-01", axioms[0].Code)
	}
	if axioms[0].Value != 1.0 {
		t.Errorf("axiom 0 value: got %f, want 1.0", axioms[0].Value)
	}
	if axioms[0].Proposition != "Any consistent formal system capable of expressing basic arithmetic contains statements that are true but unprovable within the system." {
		t.Errorf("axiom 0 proposition: got %q", axioms[0].Proposition)
	}
	if len(axioms[0].Anchors) != 4 {
		t.Errorf("axiom 0 anchors: got %d, want 4", len(axioms[0].Anchors))
	}

	// MATH-02
	if axioms[1].Code != "MATH-02" {
		t.Errorf("axiom 1 code: got %q, want MATH-02", axioms[1].Code)
	}
	if axioms[1].Value != 1.0 {
		t.Errorf("axiom 1 value: got %f, want 1.0", axioms[1].Value)
	}

	// FALSE-01
	if axioms[2].Code != "FALSE-01" {
		t.Errorf("axiom 2 code: got %q, want FALSE-01", axioms[2].Code)
	}
	if axioms[2].Value != 0.0 {
		t.Errorf("axiom 2 value: got %f, want 0.0", axioms[2].Value)
	}
	if axioms[2].Proposition != "Local hidden variable theories can account for all predictions of quantum mechanics." {
		t.Errorf("axiom 2 proposition: got %q", axioms[2].Proposition)
	}
}

func TestParseAxiomsFullFile(t *testing.T) {
	data, err := readFactsFile()
	if err != nil {
		t.Skip("FACTS.md not found, skipping integration test")
	}

	axioms := ParseAxioms(string(data))

	// Should have 23 axioms total (20 true + 3 false based on FACTS.md)
	if len(axioms) < 20 {
		t.Errorf("expected at least 20 axioms from FACTS.md, got %d", len(axioms))
	}

	// Count true vs false
	var trueCount, falseCount int
	for _, ax := range axioms {
		if ax.Value == 1.0 {
			trueCount++
		} else {
			falseCount++
		}
	}

	if falseCount != 3 {
		t.Errorf("expected 3 FALSE axioms, got %d", falseCount)
	}
	if trueCount < 17 {
		t.Errorf("expected at least 17 TRUE axioms, got %d", trueCount)
	}

	// Every axiom should have a proposition
	for _, ax := range axioms {
		if ax.Proposition == "" {
			t.Errorf("axiom %s has empty proposition", ax.Code)
		}
	}
}

func readFactsFile() ([]byte, error) {
	// Try relative paths that work from both repo root and package dir
	for _, path := range []string{"../../FACTS.md", "FACTS.md"} {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("FACTS.md not found")
}
