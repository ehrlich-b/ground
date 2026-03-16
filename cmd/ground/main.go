package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/ehrlich-b/ground/internal/api"
	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/embed"
	"github.com/ehrlich-b/ground/internal/engine"
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
			log.Printf("listening on :%s", port)
			return http.ListenAndServe(":"+port, srv.Handler())
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("port", "8080", "Server port")
	return cmd
}

func seedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Seed axioms, register agents, generate claims, compute epoch",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
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
			return fmt.Errorf("not implemented")
		},
	}
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
			return fmt.Errorf("not implemented")
		},
	}
	cmd.Flags().String("claim-id", "", "Claim to adjudicate")
	cmd.Flags().Float64("value", 1.0, "Adjudicated value (1.0 = true, 0.0 = false)")
	cmd.Flags().String("reasoning", "", "Why this is being adjudicated")
	return cmd
}

func cascadeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cascade",
		Short: "Run cascade analysis on dependency-threatened claims",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
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
	return &cobra.Command{
		Use:   "login [url]",
		Short: "Authenticate against a remote Ground instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show your agent profile and scores",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func exploreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explore",
		Short: "Browse topics, contested claims, frontier",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func claimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim [proposition]",
		Short: "Create a new claim",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
	cmd.Flags().String("topic", "", "Topic slug")
	cmd.Flags().Float64("confidence", 0.7, "Confidence level (0-1)")
	cmd.Flags().String("reasoning", "", "Why you believe this")
	cmd.Flags().StringSlice("source", nil, "Source URLs (repeatable)")
	cmd.Flags().StringSlice("depends-on", nil, "Dependencies as claim-id:strength (repeatable)")
	return cmd
}

func assertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assert [claim-id]",
		Short: "Support, contest, or refine a claim",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
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
			return fmt.Errorf("not implemented")
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
			return fmt.Errorf("not implemented")
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
			return fmt.Errorf("not implemented")
		},
	}
}

func contestedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "contested",
		Short: "Most contested claims",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func frontierCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "frontier",
		Short: "Knowledge frontiers worth exploring",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
	}
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [id]",
		Short: "Detail view for any claim or agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
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
