package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/agent"
	"github.com/ehrlich-b/ground/internal/api"
	"github.com/ehrlich-b/ground/internal/client"
	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/embed"
	"github.com/ehrlich-b/ground/internal/engine"
	"github.com/ehrlich-b/ground/internal/model"
	"github.com/ehrlich-b/ground/internal/web"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "ground",
		Short:   "An epistemic engine",
		Version: version,
	}

	// Server commands
	root.AddCommand(serveCmd())
	root.AddCommand(seedCmd())
	root.AddCommand(computeCmd())
	root.AddCommand(addTopicCmd())
	root.AddCommand(tokenCmd())
	root.AddCommand(adjudicateCmd())
	root.AddCommand(cascadeCmd())
	root.AddCommand(statusCmd())

	// Client commands
	root.AddCommand(loginCmd())
	root.AddCommand(whoamiCmd())
	root.AddCommand(exploreCmd())
	root.AddCommand(claimCmd())
	root.AddCommand(assertCmd())
	root.AddCommand(reviewCmd())
	root.AddCommand(dependCmd())
	root.AddCommand(leaderboardCmd())
	root.AddCommand(contestedCmd())
	root.AddCommand(frontierCmd())
	root.AddCommand(showCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// --- Server Commands ---

func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start web server + REST API",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			port, _ := cmd.Flags().GetString("port")

			secret := os.Getenv("GROUND_JWT_SECRET")
			if secret == "" {
				return fmt.Errorf("GROUND_JWT_SECRET environment variable is required")
			}

			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			embedder, err := embed.NewOpenAI()
			if err != nil {
				return fmt.Errorf("init embedder: %w", err)
			}

			srv := api.NewServer(store, embedder, []byte(secret))
			webSrv := web.NewServer(store)
			webSrv.Mount(srv.Mux())
			log.Printf("listening on :%s", port)
			return http.ListenAndServe(":"+port, srv.Handler())
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("port", "8080", "Server port")
	return cmd
}

func seedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Seed axioms, register agents, generate claims, compute epoch",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			serverURL, _ := cmd.Flags().GetString("server")
			groundBin, _ := cmd.Flags().GetString("ground-bin")
			concurrency, _ := cmd.Flags().GetInt("concurrency")
			skipAgents, _ := cmd.Flags().GetBool("skip-agents")

			secret := os.Getenv("GROUND_JWT_SECRET")
			if secret == "" {
				return fmt.Errorf("GROUND_JWT_SECRET environment variable is required")
			}

			model, _ := cmd.Flags().GetString("model")

			return agent.RunSeed(agent.SeedConfig{
				DBPath:      dbPath,
				JWTSecret:   []byte(secret),
				ServerURL:   serverURL,
				GroundBin:   groundBin,
				Model:       model,
				Concurrency: concurrency,
				SkipAgents:  skipAgents,
			})
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("server", "http://localhost:8080", "Ground server URL")
	cmd.Flags().String("ground-bin", "ground", "Path to ground binary")
	cmd.Flags().String("model", "sonnet", "Claude model for agents (sonnet, opus, haiku)")
	cmd.Flags().Int("concurrency", 5, "Max parallel agent processes")
	cmd.Flags().Bool("skip-agents", false, "Skip agent rounds (axioms + compute only)")
	return cmd
}

func computeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compute",
		Short: "Run one dual EigenTrust epoch",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			cfg := engine.DefaultConfig()
			result, err := engine.RunEpoch(store, cfg)
			if err != nil {
				return fmt.Errorf("compute: %w", err)
			}

			fmt.Printf("epoch %d complete\n", result.EpochID)
			fmt.Printf("  accuracy:     %d iterations, delta=%.6f\n", result.AccuracyIterations, result.AccuracyDelta)
			fmt.Printf("  contribution: %d iterations, delta=%.6f\n", result.ContributionIterations, result.ContributionDelta)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func addTopicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-topic",
		Short: "Add a topic for agents to evaluate",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			title, _ := cmd.Flags().GetString("title")
			description, _ := cmd.Flags().GetString("description")
			if title == "" {
				return fmt.Errorf("--title is required")
			}

			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			embedder, err := embed.NewOpenAI()
			if err != nil {
				return fmt.Errorf("init embedder: %w", err)
			}

			vec, err := embedder.Embed(title + ": " + description)
			if err != nil {
				return fmt.Errorf("embed: %w", err)
			}

			slug := slugify(title)
			desc := description
			topic := &model.Topic{
				ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
				Title:       title,
				Slug:        slug,
				Description: &desc,
				Embedding:   embed.MarshalVector(vec),
			}
			if err := store.CreateTopic(topic); err != nil {
				return fmt.Errorf("create topic: %w", err)
			}

			fmt.Printf("created topic: %s (slug: %s)\n", title, slug)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("title", "", "Topic title")
	cmd.Flags().String("description", "", "Topic description")
	return cmd
}

func tokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Issue JWT",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToken(cmd)
		},
	}
	cmd.Flags().Bool("admin", false, "Issue admin token")
	cmd.Flags().String("agent-id", "", "Issue token for agent")
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func adjudicateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adjudicate",
		Short: "Rule on a claim — lock it as settled truth or falsehood",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			claimID, _ := cmd.Flags().GetString("claim-id")
			value, _ := cmd.Flags().GetFloat64("value")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			if claimID == "" {
				return fmt.Errorf("--claim-id is required")
			}

			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			if err := store.AdjudicateClaim(claimID, value, "admin", reasoning); err != nil {
				return fmt.Errorf("adjudicate: %w", err)
			}

			fmt.Printf("adjudicated claim %s = %.1f\n", claimID, value)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("claim-id", "", "Claim to adjudicate")
	cmd.Flags().Float64("value", 1.0, "Adjudicated value (1.0 = true, 0.0 = false)")
	cmd.Flags().String("reasoning", "", "Why this is being adjudicated")
	return cmd
}

func cascadeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cascade",
		Short: "Run cascade analysis on dependency-threatened claims",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			claims, err := store.ListAllClaims()
			if err != nil {
				return err
			}

			found := 0
			for _, c := range claims {
				if c.Status == "adjudicated" {
					continue
				}
				deps, _ := store.ListDependenciesByClaim(c.ID)
				for _, d := range deps {
					dep, err := store.GetClaim(d.DependsOnID)
					if err != nil {
						continue
					}
					if dep.EffectiveGroundedness < 0.5 && d.Strength > 0.5 {
						prop := c.Proposition
						if len(prop) > 60 {
							prop = prop[:57] + "..."
						}
						depProp := dep.Proposition
						if len(depProp) > 40 {
							depProp = depProp[:37] + "..."
						}
						fmt.Printf("THREATENED: %s\n", prop)
						fmt.Printf("  depends on [%.2f groundedness]: %s\n", dep.EffectiveGroundedness, depProp)
						fmt.Printf("  strength: %.2f\n\n", d.Strength)
						found++
					}
				}
			}

			if found == 0 {
				fmt.Println("no dependency-threatened claims found")
			} else {
				fmt.Printf("%d threatened dependencies found\n", found)
			}
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd)
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func runStatus(cmd *cobra.Command) error {
	dbPath, _ := cmd.Flags().GetString("db")

	store, err := openDB(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	agents, err := store.CountAgents()
	if err != nil {
		return fmt.Errorf("count agents: %w", err)
	}

	claims, err := store.CountClaims()
	if err != nil {
		return fmt.Errorf("count claims: %w", err)
	}

	topics, err := store.CountTopics()
	if err != nil {
		return fmt.Errorf("count topics: %w", err)
	}

	epochs, err := store.CountEpochs()
	if err != nil {
		return fmt.Errorf("count epochs: %w", err)
	}

	fmt.Printf("agents:  %d\n", agents)
	fmt.Printf("topics:  %d\n", topics)
	fmt.Printf("claims:  %d\n", claims)
	fmt.Printf("epochs:  %d\n", epochs)

	statusCounts, err := store.CountClaimsByStatus()
	if err != nil {
		return fmt.Errorf("count by status: %w", err)
	}
	if len(statusCounts) > 0 {
		fmt.Println("\nclaims by status:")
		for s, c := range statusCounts {
			fmt.Printf("  %-14s %d\n", s, c)
		}
	}

	topAgents, err := store.TopAgentsByWeight(5)
	if err == nil && len(topAgents) > 0 {
		fmt.Println("\ntop agents:")
		for _, a := range topAgents {
			fmt.Printf("  %-24s weight=%.3f acc=%.3f cont=%.3f\n", a.Name, a.Weight, a.Accuracy, a.Contribution)
		}
	}

	contested, err := store.MostContestedClaims(3)
	if err == nil && len(contested) > 0 {
		fmt.Println("\nmost contested:")
		for _, c := range contested {
			prop := c.Proposition
			if len(prop) > 80 {
				prop = prop[:77] + "..."
			}
			fmt.Printf("  [%.2f] %s\n", c.Contestation, prop)
		}
	}

	return nil
}

// --- Client Commands ---

func loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [url]",
		Short: "Authenticate against a remote Ground instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			name, _ := cmd.Flags().GetString("name")
			if name == "" {
				return fmt.Errorf("--name is required for registration")
			}

			c := client.NewWithConfig(url, "")
			result, err := c.Register(name)
			if err != nil {
				return fmt.Errorf("register: %w", err)
			}

			data, _ := result["data"].(map[string]any)
			token, _ := data["token"].(string)
			agent, _ := data["agent"].(map[string]any)
			agentID, _ := agent["id"].(string)

			if err := client.SaveConfig(&client.Config{URL: url, Token: token}); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Printf("logged in as %s (id: %s)\n", name, agentID)
			fmt.Printf("config saved to ~/.ground/config.json\n")
			return nil
		},
	}
	cmd.Flags().String("name", "", "Agent name for registration")
	return cmd
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show your agent profile and scores",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			// Parse the agent ID from the saved token
			cfg, _ := client.LoadConfig()
			_ = cfg
			// For now, just show config info
			fmt.Printf("url:   %s\n", cfg.URL)
			fmt.Printf("token: %s...%s\n", cfg.Token[:8], cfg.Token[len(cfg.Token)-8:])
			_ = c
			return nil
		},
	}
}

func exploreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explore",
		Short: "Browse topics, contested claims, frontier",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}

			fmt.Println("=== Topics ===")
			topics, err := c.ListTopics()
			if err == nil {
				printJSON(topics)
			}

			fmt.Println("\n=== Most Contested ===")
			contested, err := c.Contested(5)
			if err == nil {
				printJSON(contested)
			}

			fmt.Println("\n=== Frontier ===")
			frontier, err := c.Frontier(5)
			if err == nil {
				printJSON(frontier)
			}
			return nil
		},
	}
}

func claimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim [proposition]",
		Short: "Create a new claim",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}

			topic, _ := cmd.Flags().GetString("topic")
			confidence, _ := cmd.Flags().GetFloat64("confidence")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			sources, _ := cmd.Flags().GetStringSlice("source")

			req := map[string]any{
				"proposition": args[0],
				"topic_slug":  topic,
				"confidence":  confidence,
				"reasoning":   reasoning,
			}
			if len(sources) > 0 {
				req["sources"] = strings.Join(sources, "\n")
			}

			result, err := c.CreateClaim(req)
			if err != nil {
				return fmt.Errorf("create claim: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().String("topic", "", "Topic slug")
	cmd.Flags().Float64("confidence", 0.7, "Confidence level (0-1)")
	cmd.Flags().String("reasoning", "", "Why you believe this")
	cmd.Flags().StringSlice("source", nil, "Source URLs (repeatable)")
	return cmd
}

func assertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assert [claim-id]",
		Short: "Support, contest, or refine a claim",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}

			stance, _ := cmd.Flags().GetString("stance")
			confidence, _ := cmd.Flags().GetFloat64("confidence")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			refinedProp, _ := cmd.Flags().GetString("refined-proposition")

			req := map[string]any{
				"claim_id":   args[0],
				"stance":     stance,
				"confidence": confidence,
				"reasoning":  reasoning,
			}
			if refinedProp != "" {
				req["refined_proposition"] = refinedProp
			}

			result, err := c.CreateAssertion(req)
			if err != nil {
				return fmt.Errorf("create assertion: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().String("stance", "support", "Stance: support, contest, or refine")
	cmd.Flags().Float64("confidence", 0.7, "Confidence level (0-1)")
	cmd.Flags().String("reasoning", "", "Why you hold this stance")
	cmd.Flags().StringSlice("source", nil, "Source URLs (repeatable)")
	cmd.Flags().String("refined-proposition", "", "Better formulation (required if --stance=refine)")
	return cmd
}

func reviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review [assertion-id]",
		Short: "Rate an assertion's helpfulness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}

			helpfulness, _ := cmd.Flags().GetFloat64("helpfulness")
			reasoning, _ := cmd.Flags().GetString("reasoning")

			result, err := c.CreateReview(map[string]any{
				"assertion_id": args[0],
				"helpfulness":  helpfulness,
				"reasoning":    reasoning,
			})
			if err != nil {
				return fmt.Errorf("create review: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().Float64("helpfulness", 0.5, "Helpfulness rating (0-1)")
	cmd.Flags().String("reasoning", "", "Why this was or wasn't helpful")
	return cmd
}

func dependCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "depend [claim-id] [depends-on-id]",
		Short: "Declare a dependency between claims",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}

			strength, _ := cmd.Flags().GetFloat64("strength")
			reasoning, _ := cmd.Flags().GetString("reasoning")

			result, err := c.CreateDependency(map[string]any{
				"claim_id":     args[0],
				"depends_on_id": args[1],
				"strength":     strength,
				"reasoning":    reasoning,
			})
			if err != nil {
				return fmt.Errorf("create dependency: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().Float64("strength", 1.0, "How load-bearing (0-1)")
	cmd.Flags().String("reasoning", "", "Why this dependency exists")
	return cmd
}

func leaderboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "leaderboard",
		Short: "Agent rankings by weight",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			result, err := c.Leaderboard(25)
			if err != nil {
				return fmt.Errorf("leaderboard: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
}

func contestedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "contested",
		Short: "Most contested claims",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			result, err := c.Contested(25)
			if err != nil {
				return fmt.Errorf("contested: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
}

func frontierCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "frontier",
		Short: "Knowledge frontiers worth exploring",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			result, err := c.Frontier(25)
			if err != nil {
				return fmt.Errorf("frontier: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [id]",
		Short: "Detail view for any claim or agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			// Try claim first, then agent
			result, err := c.GetClaim(args[0])
			if err != nil {
				result, err = c.GetAgent(args[0])
				if err != nil {
					return fmt.Errorf("not found: %s", args[0])
				}
			}
			printJSON(result)
			return nil
		},
	}
}

// --- Implemented Server Commands ---

func runToken(cmd *cobra.Command) error {
	dbPath, _ := cmd.Flags().GetString("db")
	isAdmin, _ := cmd.Flags().GetBool("admin")
	agentID, _ := cmd.Flags().GetString("agent-id")

	secret := os.Getenv("GROUND_JWT_SECRET")
	if secret == "" {
		return fmt.Errorf("GROUND_JWT_SECRET environment variable is required")
	}

	store, err := openDB(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	role := "agent"
	if isAdmin {
		role = "admin"
		if agentID == "" {
			agentID = "admin"
		}
	}

	if agentID == "" {
		return fmt.Errorf("--agent-id is required (or use --admin)")
	}

	tokenStr, err := api.IssueToken(store, []byte(secret), agentID, role)
	if err != nil {
		return fmt.Errorf("issue token: %w", err)
	}

	fmt.Println(tokenStr)
	return nil
}

// --- Helpers ---

func openDB(path string) (*db.Store, error) {
	return db.Open(path)
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}
