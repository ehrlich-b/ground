// Package sources fetches, caches, and tags evidence bodies. Every Ground citation must
// point at a source whose body is content-addressed under ~/.ground/blobs/{sha256}.
package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

// BlobStore is content-addressed storage for source bodies.
type BlobStore interface {
	Put(body []byte) (sha string, blobID string, err error)
	Get(blobID string) ([]byte, error)
}

// FileBlobStore writes blobs to ~/.ground/blobs/{sha256}.
type FileBlobStore struct {
	root string
}

func NewFileBlobStore() (*FileBlobStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	root := filepath.Join(home, ".ground", "blobs")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create blob dir: %w", err)
	}
	return &FileBlobStore{root: root}, nil
}

// NewBlobStoreAt creates a FileBlobStore rooted at a specific directory (used in tests).
func NewBlobStoreAt(root string) (*FileBlobStore, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create blob dir: %w", err)
	}
	return &FileBlobStore{root: root}, nil
}

func (b *FileBlobStore) Put(body []byte) (string, string, error) {
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	path := filepath.Join(b.root, sha)
	if _, err := os.Stat(path); err == nil {
		return sha, sha, nil
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		return "", "", fmt.Errorf("write blob: %w", err)
	}
	return sha, sha, nil
}

func (b *FileBlobStore) Get(blobID string) ([]byte, error) {
	return os.ReadFile(filepath.Join(b.root, blobID))
}

// Fetcher pulls a URL and returns extracted text body + a content-type hint.
type Fetcher interface {
	Fetch(url string) (body []byte, contentType string, err error)
}

// HTTPFetcher does a simple HTTP GET. It does NOT do readability extraction or PDF
// parsing yet; that's Phase 2 follow-up work. For MVP it returns the raw response body.
type HTTPFetcher struct {
	Client  *http.Client
	Timeout time.Duration
}

func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		Client:  &http.Client{Timeout: 30 * time.Second},
		Timeout: 30 * time.Second,
	}
}

func (f *HTTPFetcher) Fetch(rawURL string) ([]byte, string, error) {
	if _, err := url.Parse(rawURL); err != nil {
		return nil, "", fmt.Errorf("parse url: %w", err)
	}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "ground/0.1 (research-agent)")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB cap
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

// Ingester wires Fetcher + BlobStore + Store to ingest a URL into Ground.
type Ingester struct {
	Store   *db.Store
	Fetcher Fetcher
	Blobs   BlobStore
}

// IngestResult is returned from Ingest. Reused indicates the source was already cached.
type IngestResult struct {
	Source *model.Source
	Body   []byte
	Reused bool
}

// Ingest fetches the URL (if not already cached), stores its body content-addressed,
// and persists a Source row. If a source with the same content_hash already exists,
// it returns that source and Reused=true.
func (in *Ingester) Ingest(rawURL string) (*IngestResult, error) {
	body, contentType, err := in.Fetcher.Fetch(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	sha, blobID, err := in.Blobs.Put(body)
	if err != nil {
		return nil, fmt.Errorf("store blob: %w", err)
	}

	if existing, err := in.Store.GetSourceByContentHash(sha); err == nil && existing != nil {
		return &IngestResult{Source: existing, Body: body, Reused: true}, nil
	}

	srcType := guessSourceType(rawURL, contentType)
	src := &model.Source{
		ID:          db.GenerateID(),
		URL:         rawURL,
		ContentHash: sha,
		BodyBlobID:  blobID,
		FetchedAt:   time.Now().UTC(),
		Type:        srcType,
	}
	if err := in.Store.CreateSource(src); err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}

	for _, tag := range autoTags(rawURL, srcType) {
		if err := in.Store.AddSourceTag(src.ID, tag); err != nil {
			return nil, fmt.Errorf("tag source: %w", err)
		}
	}

	return &IngestResult{Source: src, Body: body, Reused: false}, nil
}

// LoadBody reads the cached body for a source.
func (in *Ingester) LoadBody(src *model.Source) ([]byte, error) {
	return in.Blobs.Get(src.BodyBlobID)
}

func guessSourceType(rawURL, contentType string) string {
	low := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(low, "https://arxiv.org/") || strings.HasPrefix(low, "http://arxiv.org/"):
		return "arxiv"
	case strings.Contains(low, "ncbi.nlm.nih.gov"):
		return "pubmed"
	case strings.HasSuffix(low, ".pdf") || strings.Contains(strings.ToLower(contentType), "pdf"):
		return "pdf"
	case strings.Contains(strings.ToLower(contentType), "text/plain"):
		return "plain"
	default:
		return "html"
	}
}

func autoTags(rawURL, srcType string) []string {
	low := strings.ToLower(rawURL)
	tags := []string{}
	switch srcType {
	case "arxiv":
		tags = append(tags, "preprint", "physics", "cs")
	case "pubmed":
		tags = append(tags, "peer-reviewed", "biomed")
	case "pdf":
		tags = append(tags, "pdf")
	}
	switch {
	case strings.HasSuffix(parseHost(low), ".gov"):
		tags = append(tags, "government")
	case strings.Contains(low, "wikipedia.org"):
		tags = append(tags, "wiki")
	case strings.Contains(low, "nature.com") || strings.Contains(low, "science.org") ||
		strings.Contains(low, "cell.com") || strings.Contains(low, "pnas.org"):
		tags = append(tags, "peer-reviewed", "tier-1-journal")
	case strings.Contains(low, "ap.org") || strings.Contains(low, "reuters.com") ||
		strings.Contains(low, "nytimes.com") || strings.Contains(low, "bbc.co.uk"):
		tags = append(tags, "news")
	case strings.Contains(low, "substack.com") || strings.Contains(low, "medium.com"):
		tags = append(tags, "blog")
	}
	return tags
}

func parseHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
