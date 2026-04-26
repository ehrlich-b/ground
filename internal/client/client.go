package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func configDir() string {
	if dir := os.Getenv("GROUND_HOME"); dir != "" {
		return filepath.Join(dir, ".ground")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ground")
}

func configPath() string { return filepath.Join(configDir(), "config.json") }

func LoadConfig() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("no config found — run 'ground login <url>' first")
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configPath(), data, 0600)
}

func New() (*Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: cfg.URL, token: cfg.Token, http: http.DefaultClient}, nil
}

func NewWithConfig(url, token string) *Client {
	return &Client{baseURL: url, token: token, http: http.DefaultClient}
}

func (c *Client) do(method, path string, body any) (map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %s", string(respBody))
	}
	if errObj, ok := result["error"]; ok {
		if errMap, ok := errObj.(map[string]any); ok {
			msg, _ := errMap["message"].(string)
			code, _ := errMap["code"].(string)
			return nil, fmt.Errorf("%s: %s", code, msg)
		}
	}
	return result, nil
}

// --- Agents ---

func (c *Client) Register(name, role string) (map[string]any, error) {
	body := map[string]any{"name": name}
	if role != "" {
		body["role"] = role
	}
	return c.do("POST", "/api/agents", body)
}

func (c *Client) GetAgent(id string) (map[string]any, error) {
	return c.do("GET", "/api/agents/"+id, nil)
}

func (c *Client) AgentLeaderboard(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/agents/leaderboard?limit=%d", limit), nil)
}

// --- Topics ---

func (c *Client) ListTopics() (map[string]any, error)         { return c.do("GET", "/api/topics", nil) }
func (c *Client) GetTopic(slug string) (map[string]any, error) { return c.do("GET", "/api/topics/"+slug, nil) }

// --- Claims ---

func (c *Client) CreateClaim(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/claims", req)
}
func (c *Client) GetClaim(id string) (map[string]any, error) { return c.do("GET", "/api/claims/"+id, nil) }
func (c *Client) ListClaims(query string) (map[string]any, error) {
	path := "/api/claims"
	if query != "" {
		path += "?" + query
	}
	return c.do("GET", path, nil)
}
func (c *Client) ClaimGradient(id string) (map[string]any, error) {
	return c.do("GET", "/api/claims/"+id+"/gradient", nil)
}

// --- Sources ---

func (c *Client) ListSources() (map[string]any, error)        { return c.do("GET", "/api/sources", nil) }
func (c *Client) GetSource(id string) (map[string]any, error) { return c.do("GET", "/api/sources/"+id, nil) }
func (c *Client) ProposeSourceCandidates(candidates []map[string]string, topicSlug string) (map[string]any, error) {
	return c.do("POST", "/api/sources/candidates", map[string]any{
		"candidates": candidates,
		"topic_slug": topicSlug,
	})
}

// --- Citations ---

func (c *Client) CreateCitation(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/citations", req)
}
func (c *Client) GetCitation(id string) (map[string]any, error) {
	return c.do("GET", "/api/citations/"+id, nil)
}

// --- Audits ---

func (c *Client) CreateAudit(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/audits", req)
}
func (c *Client) AuditQueue(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/audits/queue?limit=%d", limit), nil)
}

// --- Dependencies ---

func (c *Client) CreateDependency(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/dependencies", req)
}

// --- Lenses ---

func (c *Client) CreateLens(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/lenses", req)
}
func (c *Client) ListLenses() (map[string]any, error)        { return c.do("GET", "/api/lenses", nil) }
func (c *Client) GetLens(slug string) (map[string]any, error) { return c.do("GET", "/api/lenses/"+slug, nil) }
func (c *Client) SetLensOverrides(slug string, req map[string]any) (map[string]any, error) {
	return c.do("PUT", "/api/lenses/"+slug+"/overrides", req)
}
func (c *Client) ForkLens(slug, newSlug, desc string) (map[string]any, error) {
	return c.do("POST", "/api/lenses/"+slug+"/fork", map[string]any{
		"slug": newSlug, "description": desc,
	})
}

// --- Discovery ---

func (c *Client) SourceLeaderboard(limit int, lensSlug string) (map[string]any, error) {
	path := fmt.Sprintf("/api/leaderboard?limit=%d", limit)
	if lensSlug != "" {
		path += "&lens=" + lensSlug
	}
	return c.do("GET", path, nil)
}

func (c *Client) Contested(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/contested?limit=%d", limit), nil)
}

func (c *Client) Frontier(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/frontier?limit=%d", limit), nil)
}
