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

// Client is an HTTP client for a remote Ground instance.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Config holds the persistent client configuration.
type Config struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

func configDir() string {
	// GROUND_HOME overrides config location (used by seed agents to avoid clobbering HOME)
	if dir := os.Getenv("GROUND_HOME"); dir != "" {
		return filepath.Join(dir, ".ground")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ground")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

// LoadConfig reads the saved config from ~/.ground/config.json.
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

// SaveConfig persists config to ~/.ground/config.json.
func SaveConfig(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configPath(), data, 0600)
}

// New creates a client from the saved config.
func New() (*Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: cfg.URL,
		token:   cfg.Token,
		http:    http.DefaultClient,
	}, nil
}

// NewWithConfig creates a client from explicit URL and token.
func NewWithConfig(url, token string) *Client {
	return &Client{
		baseURL: url,
		token:   token,
		http:    http.DefaultClient,
	}
}

// do sends an HTTP request and returns the parsed JSON response body.
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

// --- API Methods ---

func (c *Client) Register(name string) (map[string]any, error) {
	return c.do("POST", "/api/agents", map[string]string{"name": name})
}

func (c *Client) GetAgent(id string) (map[string]any, error) {
	return c.do("GET", "/api/agents/"+id, nil)
}

func (c *Client) ListTopics() (map[string]any, error) {
	return c.do("GET", "/api/topics", nil)
}

func (c *Client) GetTopic(slug string) (map[string]any, error) {
	return c.do("GET", "/api/topics/"+slug, nil)
}

func (c *Client) CreateClaim(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/claims", req)
}

func (c *Client) GetClaim(id string) (map[string]any, error) {
	return c.do("GET", "/api/claims/"+id, nil)
}

func (c *Client) ListClaims(params string) (map[string]any, error) {
	path := "/api/claims"
	if params != "" {
		path += "?" + params
	}
	return c.do("GET", path, nil)
}

func (c *Client) CreateAssertion(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/assertions", req)
}

func (c *Client) CreateReview(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/reviews", req)
}

func (c *Client) CreateDependency(req map[string]any) (map[string]any, error) {
	return c.do("POST", "/api/dependencies", req)
}

func (c *Client) Leaderboard(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/leaderboard?limit=%d", limit), nil)
}

func (c *Client) Contested(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/contested?limit=%d", limit), nil)
}

func (c *Client) Frontier(limit int) (map[string]any, error) {
	return c.do("GET", fmt.Sprintf("/api/frontier?limit=%d", limit), nil)
}
