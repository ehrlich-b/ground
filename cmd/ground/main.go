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
	"github.com/ehrlich-b/ground/internal/sources"
	"github.com/ehrlich-b/ground/internal/web"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:     "ground",
		Short:   "A source-anchored encyclopedia of weighted facts",
		Version: version,
	}

	// Server
	root.AddCommand(serveCmd())
	root.AddCommand(computeCmd())
	root.AddCommand(addTopicCmd())
	root.AddCommand(tokenCmd())
	root.AddCommand(adjudicateCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(anchorCmd())
	root.AddCommand(sourceCmd())
	root.AddCommand(bootstrapAnchorsCmd())
	root.AddCommand(bootstrapAxiomsCmd())

	// Client
	root.AddCommand(loginCmd())
	root.AddCommand(whoamiCmd())
	root.AddCommand(exploreCmd())
	root.AddCommand(claimCmd())
	root.AddCommand(citeCmd())
	root.AddCommand(auditCmd())
	root.AddCommand(dependCmd())
	root.AddCommand(lensCmd())
	root.AddCommand(leaderboardCmd())
	root.AddCommand(contestedCmd())
	root.AddCommand(frontierCmd())
	root.AddCommand(showCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// --- Server commands ---

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
			blobs, err := sources.NewFileBlobStore()
			if err != nil {
				return fmt.Errorf("init blob store: %w", err)
			}
			loadBody := func(src *model.Source) ([]byte, error) { return blobs.Get(src.BodyBlobID) }
			webSrv := web.NewServer(store, loadBody)
			webSrv.Mount(srv.Mux())
			log.Printf("listening on :%s", port)
			return http.ListenAndServe(":"+port, srv.Handler())
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("port", "8080", "Server port")
	return cmd
}

func computeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compute",
		Short: "Run one epoch: source credibility, agent reliability, claim groundedness",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			result, err := engine.RunEpoch(store, engine.DefaultConfig())
			if err != nil {
				return fmt.Errorf("compute: %w", err)
			}
			fmt.Printf("epoch %d complete\n", result.EpochID)
			fmt.Printf("  source credibility:  %d iterations, delta=%.6f\n", result.SourceIterations, result.SourceDelta)
			fmt.Printf("  agent reliability:   %d iterations, delta=%.6f\n", result.AgentIterations, result.AgentDelta)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func addTopicCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-topic",
		Short: "Add a topic",
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
				ID:          db.GenerateID(),
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
		Short: "Issue JWT for an agent (admin only)",
		RunE: func(cmd *cobra.Command, args []string) error {
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
					if _, err := agent.EnsureAdminAgent(store); err != nil {
						return err
					}
					agentID = "admin"
				}
			}
			if agentID == "" {
				return fmt.Errorf("--agent-id is required (or use --admin)")
			}
			tok, err := api.IssueToken(store, []byte(secret), agentID, role)
			if err != nil {
				return fmt.Errorf("issue token: %w", err)
			}
			fmt.Println(tok)
			return nil
		},
	}
	cmd.Flags().Bool("admin", false, "Issue admin token")
	cmd.Flags().String("agent-id", "", "Agent to issue token for")
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func adjudicateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adjudicate",
		Short: "Pin a claim as adjudicated true/false (requires ≥1 citation)",
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
			citations, _ := store.ListCitationsByClaim(claimID)
			if len(citations) == 0 {
				return fmt.Errorf("cannot adjudicate %s: no citations attached", claimID)
			}
			if err := store.AdjudicateClaim(claimID, value, "admin", reasoning); err != nil {
				return fmt.Errorf("adjudicate: %w", err)
			}
			fmt.Printf("adjudicated claim %s = %.1f\n", claimID, value)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().String("claim-id", "", "Claim id")
	cmd.Flags().Float64("value", 1.0, "Adjudicated value (1.0 true, 0.0 false)")
	cmd.Flags().String("reasoning", "", "Why this is being adjudicated")
	return cmd
}

func statusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show stats summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			agents, _ := store.CountAgents()
			topics, _ := store.CountTopics()
			claims, _ := store.CountClaims()
			srcs, _ := store.CountSources()
			cits, _ := store.CountCitations()
			epochs, _ := store.CountEpochs()
			fmt.Printf("agents:     %d\n", agents)
			fmt.Printf("topics:     %d\n", topics)
			fmt.Printf("sources:    %d\n", srcs)
			fmt.Printf("claims:     %d\n", claims)
			fmt.Printf("citations:  %d\n", cits)
			fmt.Printf("epochs:     %d\n", epochs)
			counts, _ := store.CountClaimsByStatus()
			if len(counts) > 0 {
				fmt.Println("\nclaims by status:")
				for s, c := range counts {
					fmt.Printf("  %-14s %d\n", s, c)
				}
			}
			top, _ := store.TopAgentsByReliability(5)
			if len(top) > 0 {
				fmt.Println("\ntop agents by reliability:")
				for _, a := range top {
					fmt.Printf("  %-24s reliability=%.3f productivity=%.3f role=%s\n", a.Name, a.Reliability, a.Productivity, a.Role)
				}
			}
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

// --- Anchor / source admin ---

func anchorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anchor",
		Short: "Manage admin-curated source anchors",
	}
	cmd.AddCommand(anchorAddCmd())
	cmd.AddCommand(anchorListCmd())
	cmd.AddCommand(anchorRemoveCmd())
	return cmd
}

func anchorAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [url]",
		Short: "Anchor a source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			tier, _ := cmd.Flags().GetInt("tier")
			cred, _ := cmd.Flags().GetFloat64("credibility")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			ing, err := newIngester(store)
			if err != nil {
				return err
			}
			res, err := ing.Ingest(args[0])
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}
			anchor := &model.SourceAnchor{
				SourceID:    res.Source.ID,
				Tier:        tier,
				Credibility: cred,
				SetBy:       "admin",
				Reasoning:   nullable(reasoning),
			}
			if err := store.UpsertSourceAnchor(anchor); err != nil {
				return fmt.Errorf("anchor: %w", err)
			}
			fmt.Printf("anchored %s (tier=%d cred=%.2f)\n", args[0], tier, cred)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	cmd.Flags().Int("tier", 2, "Anchor tier (1-4)")
	cmd.Flags().Float64("credibility", 0.7, "Initial credibility prior")
	cmd.Flags().String("reasoning", "", "Why this is anchored")
	return cmd
}

func anchorListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List anchored sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			anchors, err := store.ListSourceAnchors()
			if err != nil {
				return err
			}
			for _, a := range anchors {
				src, _ := store.GetSource(a.SourceID)
				url := a.SourceID
				if src != nil {
					url = src.URL
				}
				fmt.Printf("[tier %d, cred %.2f] %s\n", a.Tier, a.Credibility, url)
			}
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func anchorRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [source-id]",
		Short: "Remove an anchor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.DeleteSourceAnchor(args[0])
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func sourceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage sources",
	}
	cmd.AddCommand(sourceIngestCmd())
	cmd.AddCommand(sourceTagCmd())
	return cmd
}

func sourceIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest [url]",
		Short: "Fetch and store a source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			ing, err := newIngester(store)
			if err != nil {
				return err
			}
			res, err := ing.Ingest(args[0])
			if err != nil {
				return fmt.Errorf("ingest: %w", err)
			}
			fmt.Printf("source %s (%s, reused=%v)\n", res.Source.ID, res.Source.Type, res.Reused)
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func sourceTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag [source-id] [tag]",
		Short: "Tag a source",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.AddSourceTag(args[0], args[1])
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func bootstrapAnchorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap-anchors [path]",
		Short: "Load anchors from anchors.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read anchors file: %w", err)
			}
			var doc struct {
				Anchors []struct {
					URL         string  `yaml:"url"`
					Tier        int     `yaml:"tier"`
					Credibility float64 `yaml:"credibility"`
					Reasoning   string  `yaml:"reasoning"`
				} `yaml:"anchors"`
			}
			if err := yaml.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("parse anchors yaml: %w", err)
			}
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			ing, err := newIngester(store)
			if err != nil {
				return err
			}
			for _, a := range doc.Anchors {
				res, err := ing.Ingest(a.URL)
				if err != nil {
					log.Printf("ingest %s: %v", a.URL, err)
					continue
				}
				if err := store.UpsertSourceAnchor(&model.SourceAnchor{
					SourceID:    res.Source.ID,
					Tier:        a.Tier,
					Credibility: a.Credibility,
					SetBy:       "admin",
					Reasoning:   nullable(a.Reasoning),
				}); err != nil {
					log.Printf("anchor %s: %v", a.URL, err)
				}
			}
			fmt.Printf("loaded %d anchors\n", len(doc.Anchors))
			return nil
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

func bootstrapAxiomsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap-axioms [path]",
		Short: "Load FACTS.md axioms as adjudicated claims",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, _ := cmd.Flags().GetString("db")
			store, err := openDB(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()
			data, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read axioms file: %w", err)
			}
			axioms := agent.ParseAxioms(string(data))
			return agent.SeedAxioms(store, axioms)
		},
	}
	cmd.Flags().String("db", "ground.db", "Database path")
	return cmd
}

// --- Client commands ---

func loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [url]",
		Short: "Authenticate against a Ground instance (registers a new agent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			name, _ := cmd.Flags().GetString("name")
			role, _ := cmd.Flags().GetString("role")
			if name == "" {
				return fmt.Errorf("--name is required for registration")
			}
			c := client.NewWithConfig(url, "")
			result, err := c.Register(name, role)
			if err != nil {
				return fmt.Errorf("register: %w", err)
			}
			data, _ := result["data"].(map[string]any)
			token, _ := data["token"].(string)
			a, _ := data["agent"].(map[string]any)
			id, _ := a["id"].(string)
			if err := client.SaveConfig(&client.Config{URL: url, Token: token}); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("logged in as %s (id: %s, role: %s)\n", name, id, role)
			return nil
		},
	}
	cmd.Flags().String("name", "", "Agent name")
	cmd.Flags().String("role", "both", "Agent role (extractor, auditor, both, observer)")
	return cmd
}

func whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show your config + recent activity",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := client.LoadConfig()
			if err != nil {
				return err
			}
			fmt.Printf("url:   %s\n", cfg.URL)
			fmt.Printf("token: %s...%s\n", cfg.Token[:8], cfg.Token[len(cfg.Token)-8:])
			return nil
		},
	}
}

func exploreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explore",
		Short: "Browse contested + frontier claims",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			fmt.Println("=== Most Contested ===")
			contested, _ := c.Contested(5)
			printJSON(contested)
			fmt.Println("\n=== Frontier ===")
			frontier, _ := c.Frontier(5)
			printJSON(frontier)
			return nil
		},
	}
}

func claimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim [proposition]",
		Short: "Create a new claim with at least one citation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			source, _ := cmd.Flags().GetString("source")
			quote, _ := cmd.Flags().GetString("quote")
			polarity, _ := cmd.Flags().GetString("polarity")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			if source == "" || quote == "" {
				return fmt.Errorf("--source and --quote are required (one citation minimum)")
			}
			req := map[string]any{
				"proposition": args[0],
				"citations": []map[string]any{
					{
						"url":            source,
						"verbatim_quote": quote,
						"polarity":       polarity,
						"reasoning":      reasoning,
					},
				},
			}
			result, err := c.CreateClaim(req)
			if err != nil {
				return fmt.Errorf("create claim: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().String("source", "", "Source URL or id for the required citation")
	cmd.Flags().String("quote", "", "Verbatim quote substring of the source body")
	cmd.Flags().String("polarity", "supports", "supports | contradicts | qualifies")
	cmd.Flags().String("reasoning", "", "Why this quote backs the claim")
	return cmd
}

func citeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cite [claim-id]",
		Short: "Propose a citation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			source, _ := cmd.Flags().GetString("source")
			quote, _ := cmd.Flags().GetString("quote")
			polarity, _ := cmd.Flags().GetString("polarity")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			req := map[string]any{
				"claim_id":       args[0],
				"verbatim_quote": quote,
				"polarity":       polarity,
				"reasoning":      reasoning,
			}
			if strings.HasPrefix(source, "http") {
				req["url"] = source
			} else {
				req["source_id"] = source
			}
			result, err := c.CreateCitation(req)
			if err != nil {
				return fmt.Errorf("create citation: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().String("source", "", "Source URL or id")
	cmd.Flags().String("quote", "", "Verbatim quote (must literally appear in the source body)")
	cmd.Flags().String("polarity", "supports", "supports | contradicts | qualifies")
	cmd.Flags().String("reasoning", "", "Why this quote backs the claim")
	return cmd
}

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit [citation-id]",
		Short: "Verify someone else's citation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			semantic, _ := cmd.Flags().GetString("semantic")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			result, err := c.CreateAudit(map[string]any{
				"citation_id": args[0],
				"semantic":    semantic,
				"reasoning":   reasoning,
			})
			if err != nil {
				return fmt.Errorf("create audit: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().String("semantic", "confirm", "confirm | misquote | out_of_context | weak | broken_link")
	cmd.Flags().String("reasoning", "", "Why this verdict")
	return cmd
}

func dependCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "depend [claim-id] [depends-on-id]",
		Short: "Declare a dependency",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			strength, _ := cmd.Flags().GetFloat64("strength")
			reasoning, _ := cmd.Flags().GetString("reasoning")
			result, err := c.CreateDependency(map[string]any{
				"claim_id":      args[0],
				"depends_on_id": args[1],
				"strength":      strength,
				"reasoning":     reasoning,
			})
			if err != nil {
				return fmt.Errorf("depend: %w", err)
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().Float64("strength", 1.0, "Dependency strength (0-1)")
	cmd.Flags().String("reasoning", "", "Why this dependency")
	return cmd
}

func lensCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lens",
		Short: "Manage lenses",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "new",
		Short: "Create a new lens",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			slug, _ := cmd.Flags().GetString("slug")
			desc, _ := cmd.Flags().GetString("description")
			result, err := c.CreateLens(map[string]any{"slug": slug, "description": desc, "public": true})
			if err != nil {
				return err
			}
			printJSON(result)
			return nil
		},
	})
	cmd.Flags().String("slug", "", "Lens slug")
	cmd.Flags().String("description", "", "Lens description")
	return cmd
}

func leaderboardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "Source credibility ranking (lens-aware)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			lensSlug, _ := cmd.Flags().GetString("lens")
			result, err := c.SourceLeaderboard(25, lensSlug)
			if err != nil {
				return err
			}
			printJSON(result)
			return nil
		},
	}
	cmd.Flags().String("lens", "", "Apply a lens slug")
	return cmd
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
				return err
			}
			printJSON(result)
			return nil
		},
	}
}

func frontierCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "frontier",
		Short: "High-fan-out, high-contestation claims",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
			result, err := c.Frontier(25)
			if err != nil {
				return err
			}
			printJSON(result)
			return nil
		},
	}
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [id]",
		Short: "Show a claim or agent by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.New()
			if err != nil {
				return err
			}
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

// --- Helpers ---

func openDB(path string) (*db.Store, error) { return db.Open(path) }

func newIngester(store *db.Store) (*sources.Ingester, error) {
	blobs, err := sources.NewFileBlobStore()
	if err != nil {
		return nil, err
	}
	return &sources.Ingester{
		Store:   store,
		Fetcher: sources.NewHTTPFetcher(),
		Blobs:   blobs,
	}, nil
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
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

var _ = time.Now
