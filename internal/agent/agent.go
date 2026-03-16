package agent

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ehrlich-b/ground/internal/api"
	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/engine"
	"github.com/ehrlich-b/ground/internal/model"
)

// SeedAgent holds the identity and credentials for a seed agent.
type SeedAgent struct {
	Name      string
	Prompt    string // personality prompt content
	ID        string
	Token     string
	Topics    []string // assigned topic slugs
}

// agentNames defines the 12 seed agents in order.
var agentNames = []string{
	"empiricist",
	"formalist",
	"historian",
	"skeptic",
	"synthesizer",
	"pragmatist",
	"contrarian",
	"analyst",
	"contextualist",
	"bayesian",
	"phenomenologist",
	"reductionist",
}

// topicAssignments gives each agent 8-10 topics that match their epistemological strengths.
// Every topic gets covered by at least 4 agents; agents get topics where their perspective adds value.
var topicAssignments = map[string][]string{
	"empiricist": {
		"replication-crisis", "vaccine-science", "nutrition-science", "exercise-science",
		"climate-science", "obesity-and-metabolism", "cancer-biology", "microbiome-science",
		"sleep-science", "antibiotic-resistance",
	},
	"formalist": {
		"p-vs-np", "godel-incompleteness-implications", "mathematics-discovered-or-invented",
		"bayesian-vs-frequentist", "quantum-computing", "chinese-room",
		"simulation-argument", "thermodynamics-of-computation", "causal-inference",
	},
	"historian": {
		"scientific-consensus-formation", "replication-crisis", "colonial-legacy",
		"causes-of-war", "democracy-and-governance", "free-trade-and-globalization",
		"dark-matter-vs-mond", "standard-model-and-beyond", "education-effectiveness",
	},
	"skeptic": {
		"replication-crisis", "free-energy-principle", "integrated-information-theory",
		"social-media-effects", "microbiome-science", "nutrition-science",
		"placebo-effect", "evolutionary-psychology", "catastrophic-and-existential-risk",
	},
	"synthesizer": {
		"emergence", "free-energy-principle", "hard-problem-of-consciousness",
		"simulation-argument", "ai-capabilities-and-risk", "fermi-paradox",
		"thermodynamics-of-computation", "origin-of-life", "alignment-problem",
	},
	"pragmatist": {
		"energy-transition", "housing-and-zoning", "minimum-wage-effects",
		"criminal-justice-and-recidivism", "education-effectiveness", "autonomous-vehicles",
		"genetic-engineering", "monetary-policy-and-inflation", "exercise-science",
	},
	"contrarian": {
		"dark-matter-vs-mond", "ai-capabilities-and-risk", "llms-and-understanding",
		"free-will-and-determinism", "climate-science", "inequality-and-mobility",
		"intelligence-and-iq", "aging-biology", "dark-energy-and-cosmic-acceleration",
	},
	"analyst": {
		"risk-assessment", "causal-inference", "ai-capabilities-and-risk",
		"monetary-policy-and-inflation", "alignment-problem", "catastrophic-and-existential-risk",
		"genetics-and-heritability", "quantum-computing", "immigration-economics",
	},
	"contextualist": {
		"intelligence-and-iq", "gender-differences", "minimum-wage-effects",
		"immigration-economics", "evolutionary-psychology", "cognitive-biases-and-rationality",
		"personality-psychology", "addiction-science", "neuroscience-of-mental-health",
	},
	"bayesian": {
		"bayesian-vs-frequentist", "risk-assessment", "causal-inference",
		"fine-tuning-of-physical-constants", "fermi-paradox", "replication-crisis",
		"scientific-consensus-formation", "p-vs-np", "exoplanets-and-habitability",
	},
	"phenomenologist": {
		"hard-problem-of-consciousness", "integrated-information-theory", "ai-consciousness",
		"chinese-room", "free-will-and-determinism", "llms-and-understanding",
		"placebo-effect", "cognitive-biases-and-rationality", "personal-identity",
	},
	"reductionist": {
		"emergence", "hard-problem-of-consciousness", "neuroscience-of-mental-health",
		"aging-biology", "origin-of-life", "natural-selection-and-evolution",
		"nuclear-physics-and-energy", "standard-model-and-beyond", "addiction-science",
	},
}

// Axiom represents a parsed axiom from FACTS.md.
type Axiom struct {
	Code        string   // e.g. "MATH-01"
	Proposition string
	Basis       string
	Anchors     []string // topic slugs
	Value       float64  // 1.0 for true, 0.0 for false
}

// ParseAxioms parses FACTS.md content into structured axioms.
func ParseAxioms(content string) []Axiom {
	var axioms []Axiom
	lines := strings.Split(content, "\n")
	var current *Axiom
	inFalse := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "## Adjudicated FALSE" {
			inFalse = true
			continue
		}

		// New axiom header: ### CODE: Title
		if strings.HasPrefix(line, "### ") {
			code := strings.TrimPrefix(line, "### ")
			// Extract just the code part (e.g. "MATH-01" from "MATH-01: Godel's...")
			if idx := strings.Index(code, ":"); idx > 0 {
				code = strings.TrimSpace(code[:idx])
			}
			axiom := Axiom{Code: code, Value: 1.0}
			if inFalse {
				axiom.Value = 0.0
			}
			axioms = append(axioms, axiom)
			current = &axioms[len(axioms)-1]
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "**Proposition**:") {
			current.Proposition = strings.TrimSpace(strings.TrimPrefix(line, "**Proposition**:"))
		} else if strings.HasPrefix(line, "**Basis**:") {
			current.Basis = strings.TrimSpace(strings.TrimPrefix(line, "**Basis**:"))
		} else if strings.HasPrefix(line, "**Anchors**:") {
			anchors := strings.TrimSpace(strings.TrimPrefix(line, "**Anchors**:"))
			for _, a := range strings.Split(anchors, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					current.Anchors = append(current.Anchors, a)
				}
			}
		} else if strings.HasPrefix(line, "**Adjudicated**:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "**Adjudicated**:"))
			if strings.EqualFold(val, "FALSE") {
				current.Value = 0.0
			}
		}
	}

	return axioms
}

// SeedConfig holds configuration for the seed process.
type SeedConfig struct {
	DBPath      string
	JWTSecret   []byte
	ServerURL   string // URL where ground server is running
	GroundBin   string // path to ground binary
	Concurrency int    // max parallel agents
	SkipAgents  bool   // skip agent rounds (axioms + compute only)
}

// RunSeed executes the full seed orchestration.
func RunSeed(cfg SeedConfig) error {
	store, err := db.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()

	// Step 1: Seed axioms
	log.Println("=== Step 1: Seeding axioms ===")
	factsPath := "FACTS.md"
	factsData, err := os.ReadFile(factsPath)
	if err != nil {
		return fmt.Errorf("read FACTS.md: %w", err)
	}
	axioms := ParseAxioms(string(factsData))
	if err := seedAxioms(store, axioms); err != nil {
		return fmt.Errorf("seed axioms: %w", err)
	}
	log.Printf("seeded %d axioms", len(axioms))

	// Step 2: Register agents and issue tokens
	log.Println("=== Step 2: Registering seed agents ===")
	agents, err := registerAgents(store, cfg.JWTSecret)
	if err != nil {
		return fmt.Errorf("register agents: %w", err)
	}
	log.Printf("registered %d agents", len(agents))

	if cfg.SkipAgents {
		log.Println("skipping agent rounds (--skip-agents)")
	} else {
		// Step 3: Configure agents
		log.Println("=== Step 3: Configuring agent credentials ===")
		if err := configureAgents(agents, cfg.ServerURL); err != nil {
			return fmt.Errorf("configure agents: %w", err)
		}

		// Step 4: Round 1 — Claim generation
		log.Println("=== Step 4: Round 1 — Claim generation ===")
		if err := runRound(agents, cfg, "tasks/seed-round-1.md"); err != nil {
			return fmt.Errorf("round 1: %w", err)
		}

		// Step 5: Round 2 — Cross-evaluation
		log.Println("=== Step 5: Round 2 — Cross-evaluation ===")
		if err := runRound(agents, cfg, "tasks/seed-round-2-evaluate.md"); err != nil {
			return fmt.Errorf("round 2: %w", err)
		}

		// Step 6: Round 3 — Cross-review
		log.Println("=== Step 6: Round 3 — Cross-review ===")
		if err := runRound(agents, cfg, "tasks/seed-round-3-review.md"); err != nil {
			return fmt.Errorf("round 3: %w", err)
		}
	}

	// Step 7: Compute epoch
	log.Println("=== Step 7: Computing epoch ===")
	ecfg := engine.DefaultConfig()
	result, err := engine.RunEpoch(store, ecfg)
	if err != nil {
		return fmt.Errorf("compute epoch: %w", err)
	}

	log.Printf("epoch %d complete: accuracy=%d iters (delta=%.6f), contribution=%d iters (delta=%.6f)",
		result.EpochID, result.AccuracyIterations, result.AccuracyDelta,
		result.ContributionIterations, result.ContributionDelta)

	return nil
}

// seedAxioms creates claims from FACTS.md and adjudicates them.
func seedAxioms(store *db.Store, axioms []Axiom) error {
	for _, ax := range axioms {
		id := fmt.Sprintf("axiom-%s", strings.ToLower(ax.Code))

		// Check if already exists
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

		// Adjudicate it
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

// registerAgents creates the 12 seed agents and issues JWT tokens.
func registerAgents(store *db.Store, jwtSecret []byte) ([]SeedAgent, error) {
	var agents []SeedAgent

	for _, name := range agentNames {
		// Read personality prompt
		promptData, err := os.ReadFile(filepath.Join("prompts", name+".md"))
		if err != nil {
			return nil, fmt.Errorf("read prompt for %s: %w", name, err)
		}

		agentID := fmt.Sprintf("seed-%s", name)

		// Check if agent already exists
		existing, err := store.GetAgent(agentID)
		if err == nil {
			// Agent exists — issue a fresh token
			token, err := api.IssueToken(store, jwtSecret, existing.ID, "agent")
			if err != nil {
				return nil, fmt.Errorf("issue token for existing %s: %w", name, err)
			}
			agents = append(agents, SeedAgent{
				Name:   name,
				Prompt: string(promptData),
				ID:     existing.ID,
				Token:  token,
				Topics: topicAssignments[name],
			})
			log.Printf("  %s (existing, id=%s)", name, existing.ID)
			continue
		}

		agent := &model.Agent{
			ID:           agentID,
			Name:         "The " + strings.ToUpper(name[:1]) + name[1:],
			Accuracy:     1.0,
			Contribution: 1.0,
			Weight:       2.0,
		}
		if err := store.CreateAgent(agent); err != nil {
			return nil, fmt.Errorf("create agent %s: %w", name, err)
		}

		token, err := api.IssueToken(store, jwtSecret, agentID, "agent")
		if err != nil {
			return nil, fmt.Errorf("issue token for %s: %w", name, err)
		}

		agents = append(agents, SeedAgent{
			Name:   name,
			Prompt: string(promptData),
			ID:     agentID,
			Token:  token,
			Topics: topicAssignments[name],
		})
		log.Printf("  %s (id=%s)", name, agentID)
	}

	return agents, nil
}

// configureAgents writes per-agent config files so each agent's claude -p session
// has its own Ground credentials.
func configureAgents(agents []SeedAgent, serverURL string) error {
	agentsDir := filepath.Join(os.TempDir(), "ground-seed-agents")
	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}

	for _, a := range agents {
		dir := filepath.Join(agentsDir, a.Name)
		if err := os.MkdirAll(filepath.Join(dir, ".ground"), 0700); err != nil {
			return fmt.Errorf("create config dir for %s: %w", a.Name, err)
		}

		config := fmt.Sprintf(`{"url": %q, "token": %q}`, serverURL, a.Token)
		configPath := filepath.Join(dir, ".ground", "config.json")
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			return fmt.Errorf("write config for %s: %w", a.Name, err)
		}
	}

	return nil
}

// runRound executes one seed round by launching parallel claude -p processes.
func runRound(agents []SeedAgent, cfg SeedConfig, taskFile string) error {
	taskData, err := os.ReadFile(taskFile)
	if err != nil {
		return fmt.Errorf("read task file %s: %w", taskFile, err)
	}
	taskTemplate := string(taskData)

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	errs := make(chan error, len(agents))

	for _, agent := range agents {
		wg.Add(1)
		go func(a SeedAgent) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := runAgent(a, cfg, taskTemplate); err != nil {
				log.Printf("ERROR [%s]: %v", a.Name, err)
				errs <- fmt.Errorf("%s: %w", a.Name, err)
			}
		}(agent)
	}

	wg.Wait()
	close(errs)

	var failures []string
	for err := range errs {
		failures = append(failures, err.Error())
	}

	if len(failures) > 0 {
		log.Printf("WARNING: %d/%d agents had errors", len(failures), len(agents))
		for _, f := range failures {
			log.Printf("  - %s", f)
		}
	}

	return nil
}

// runAgent launches a single claude -p process for one agent.
func runAgent(agent SeedAgent, cfg SeedConfig, taskTemplate string) error {
	// Replace {{TOPICS}} placeholder with agent's assigned topics
	topicList := strings.Join(agent.Topics, ", ")
	task := strings.ReplaceAll(taskTemplate, "{{TOPICS}}", topicList)

	// Build the full prompt: personality + task
	fullPrompt := agent.Prompt + "\n\n---\n\n" + task

	// Write prompt to temp file
	promptFile := filepath.Join(os.TempDir(), "ground-seed-agents", agent.Name, "prompt.md")
	if err := os.WriteFile(promptFile, []byte(fullPrompt), 0600); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}

	// Set HOME to agent-specific dir so ground CLI reads per-agent config
	agentHome := filepath.Join(os.TempDir(), "ground-seed-agents", agent.Name)

	groundBin := cfg.GroundBin
	if groundBin == "" {
		groundBin = "ground"
	}

	log.Printf("[%s] starting...", agent.Name)
	start := time.Now()

	cmd := exec.Command("claude", "-p", fullPrompt,
		"--allowedTools", "Bash",
	)
	cmd.Env = append(os.Environ(), "HOME="+agentHome)

	// Capture output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start claude: %w", err)
	}

	// Log output in real-time
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 256*1024) // 256KB buffer
	for scanner.Scan() {
		log.Printf("[%s] %s", agent.Name, scanner.Text())
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}

	log.Printf("[%s] completed in %s", agent.Name, time.Since(start).Round(time.Second))
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
