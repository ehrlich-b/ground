package sources_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/sources"
)

func newIngester(t *testing.T) *sources.Ingester {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	blobs, err := sources.NewBlobStoreAt(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatalf("blobs: %v", err)
	}
	return &sources.Ingester{Store: store, Blobs: blobs}
}

type fixedFetcher struct {
	body        []byte
	contentType string
}

func (f *fixedFetcher) Fetch(_ string) (*sources.FetchResult, error) {
	return &sources.FetchResult{Raw: f.body, ContentType: f.contentType}, nil
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func TestExtractHTMLStripsChrome(t *testing.T) {
	raw := loadFixture(t, "article.html")
	body, err := sources.ExtractBody(raw, "text/html", "https://example.com/landauer", "html")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if body.Unverifiable {
		t.Fatalf("expected verifiable body, got unverifiable")
	}
	text := string(body.Text)
	if !strings.Contains(text, "approaches the Landauer limit") {
		t.Errorf("expected article prose in extracted text; got %d chars", len(text))
	}
	for _, junk := range []string{"SPONSORED", "Sign in", "$9.99", "tracker.send"} {
		if strings.Contains(text, junk) {
			t.Errorf("chrome leaked into extracted text: %q", junk)
		}
	}
}

func TestExtractPaywallFlagsUnverifiable(t *testing.T) {
	raw := loadFixture(t, "paywall.html")
	body, err := sources.ExtractBody(raw, "text/html", "https://example.com/premium", "html")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !body.Unverifiable {
		t.Errorf("expected paywall stub to be unverifiable")
	}
}

func TestIngestStoresExtractedText(t *testing.T) {
	in := newIngester(t)
	raw := loadFixture(t, "article.html")
	in.Fetcher = &fixedFetcher{body: raw, contentType: "text/html"}

	res, err := in.Ingest("https://example.com/landauer")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Source.Type != "html" {
		t.Errorf("expected type=html, got %q", res.Source.Type)
	}
	if !db.HasSourceQuote(string(res.Body), "approaches the Landauer limit") {
		t.Errorf("mechanical wall failed against extracted body")
	}
	if db.HasSourceQuote(string(res.Body), "SPONSORED: Buy our newsletter") {
		t.Errorf("ad text leaked through extraction")
	}
}

func TestIngestPaywallTaggedUnverifiable(t *testing.T) {
	in := newIngester(t)
	in.Fetcher = &fixedFetcher{body: loadFixture(t, "paywall.html"), contentType: "text/html"}
	res, err := in.Ingest("https://example.com/premium")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if res.Source.Type != "unverifiable" {
		t.Errorf("expected type=unverifiable, got %q", res.Source.Type)
	}
}

func TestIngestDedupsByContentHash(t *testing.T) {
	in := newIngester(t)
	raw := loadFixture(t, "article.html")
	in.Fetcher = &fixedFetcher{body: raw, contentType: "text/html"}

	first, err := in.Ingest("https://example.com/a")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := in.Ingest("https://example.com/b")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.Reused {
		t.Errorf("expected dedup hit on identical content")
	}
	if first.Source.ID != second.Source.ID {
		t.Errorf("dedup returned different source ids")
	}
}

func TestHTTPFetcherIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body><article><h1>Hi</h1><p>" + strings.Repeat("This is a test article about reversible computing. ", 20) + "</p></article></body></html>"))
	}))
	defer srv.Close()

	f := sources.NewHTTPFetcher()
	res, err := f.Fetch(srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(res.ContentType, "text/html") {
		t.Errorf("expected text/html content type, got %q", res.ContentType)
	}
	if !strings.Contains(string(res.Raw), "reversible computing") {
		t.Errorf("expected fetched body to contain article text")
	}
}
