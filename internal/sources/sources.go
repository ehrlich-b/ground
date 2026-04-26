// Package sources fetches, caches, and tags evidence bodies. Every Ground citation must
// point at a source whose body is content-addressed under ~/.ground/blobs/{sha256}.
package sources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	nurl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
	readability "github.com/go-shiori/go-readability"
)

// MinReadableTextLength is the threshold below which extracted text is treated
// as a likely paywall/JS-wall and the source is flagged unverifiable.
const MinReadableTextLength = 400

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

// FetchResult is the raw output of a Fetcher.
type FetchResult struct {
	Raw         []byte
	ContentType string
}

// Fetcher pulls a URL and returns the raw response bytes + a content-type hint.
type Fetcher interface {
	Fetch(url string) (*FetchResult, error)
}

// HTTPFetcher does an HTTP GET with a timeout and a 10MB cap.
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

func (f *HTTPFetcher) Fetch(rawURL string) (*FetchResult, error) {
	if _, err := nurl.Parse(rawURL); err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "ground/0.1 (research-agent)")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return &FetchResult{Raw: body, ContentType: resp.Header.Get("Content-Type")}, nil
}

// ExtractedBody is the readable text used as the source body for the mechanical
// wall, plus optional metadata recovered during extraction.
type ExtractedBody struct {
	Text     []byte
	Title    string
	Excerpt  string
	SiteName string
	// Unverifiable marks pages whose readable text was below the paywall
	// threshold. Citations against unverifiable bodies still need to mechanically
	// match; the engine separately floors their credibility.
	Unverifiable bool
}

// ExtractBody turns raw fetched bytes into the canonical text body that
// citation verbatim quotes are checked against. HTML runs through readability;
// PDFs and unknown types are passed through verbatim for now.
func ExtractBody(raw []byte, contentType, rawURL, srcType string) (*ExtractedBody, error) {
	switch srcType {
	case "html", "arxiv", "pubmed":
		return extractHTML(raw, rawURL)
	case "plain":
		return &ExtractedBody{Text: raw}, nil
	default:
		return &ExtractedBody{Text: raw}, nil
	}
}

func extractHTML(raw []byte, rawURL string) (*ExtractedBody, error) {
	pageURL, err := nurl.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	article, err := readability.FromReader(bytes.NewReader(raw), pageURL)
	if err != nil {
		return &ExtractedBody{Text: raw, Unverifiable: true}, nil
	}
	text := strings.TrimSpace(article.TextContent)
	if text == "" {
		return &ExtractedBody{Text: raw, Unverifiable: true}, nil
	}
	out := &ExtractedBody{
		Text:     []byte(text),
		Title:    strings.TrimSpace(article.Title),
		Excerpt:  strings.TrimSpace(article.Excerpt),
		SiteName: strings.TrimSpace(article.SiteName),
	}
	if len(out.Text) < MinReadableTextLength {
		out.Unverifiable = true
	}
	return out, nil
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

// Ingest fetches the URL (if not already cached), runs body extraction so the
// mechanical wall checks against readable text rather than raw HTML, stores the
// extracted body content-addressed, and persists a Source row. Identical
// extracted bodies dedup by content_hash even across different URLs.
func (in *Ingester) Ingest(rawURL string) (*IngestResult, error) {
	fetched, err := in.Fetcher.Fetch(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	srcType := guessSourceType(rawURL, fetched.ContentType)
	extracted, err := ExtractBody(fetched.Raw, fetched.ContentType, rawURL, srcType)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}

	sha, blobID, err := in.Blobs.Put(extracted.Text)
	if err != nil {
		return nil, fmt.Errorf("store blob: %w", err)
	}

	if existing, err := in.Store.GetSourceByContentHash(sha); err == nil && existing != nil {
		return &IngestResult{Source: existing, Body: extracted.Text, Reused: true}, nil
	}

	if extracted.Unverifiable {
		srcType = "unverifiable"
	}
	src := &model.Source{
		ID:          db.GenerateID(),
		URL:         rawURL,
		ContentHash: sha,
		BodyBlobID:  blobID,
		FetchedAt:   time.Now().UTC(),
		Type:        srcType,
	}
	if extracted.Title != "" {
		t := extracted.Title
		src.Title = &t
	}
	if err := in.Store.CreateSource(src); err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}

	for _, tag := range autoTags(rawURL, srcType) {
		if err := in.Store.AddSourceTag(src.ID, tag); err != nil {
			return nil, fmt.Errorf("tag source: %w", err)
		}
	}

	return &IngestResult{Source: src, Body: extracted.Text, Reused: false}, nil
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
	u, err := nurl.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
