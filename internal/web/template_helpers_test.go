package web

import (
	"strings"
	"testing"
	"time"

	"github.com/ehrlich-b/ground/internal/model"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"shorter than limit is unchanged", "hello", 10, "hello"},
		{"exactly at limit is unchanged", "hello", 5, "hello"},
		{"longer than limit truncates with ellipsis", "hello world", 8, "hello..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.s, tt.n); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestStatusClass(t *testing.T) {
	known := []string{"grounded", "adjudicated", "contested", "active", "emerging", "refuted"}
	for _, s := range known {
		if got := statusClass(s); got != s {
			t.Errorf("statusClass(%q) = %q, want %q", s, got, s)
		}
	}
	for _, s := range []string{"unknown", ""} {
		if got := statusClass(s); got != "active" {
			t.Errorf("statusClass(%q) = %q, want %q", s, got, "active")
		}
	}
}

func TestBarClassAndColorForValue(t *testing.T) {
	bands := []struct {
		v       float64
		wantBar string
		wantCol string
	}{
		{0.7, "high", "var(--accent)"},
		{0.4, "medium", "var(--yellow)"},
		{0.39, "low", "var(--red)"},
		{1.0, "high", "var(--accent)"},
		{0.5, "medium", "var(--yellow)"},
		{0.0, "low", "var(--red)"},
	}
	for _, b := range bands {
		if got := barClass(b.v); got != b.wantBar {
			t.Errorf("barClass(%v) = %q, want %q", b.v, got, b.wantBar)
		}
		if got := colorForValue(b.v); got != b.wantCol {
			t.Errorf("colorForValue(%v) = %q, want %q", b.v, got, b.wantCol)
		}
	}
}

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{"under a minute is just now", 30 * time.Second, "just now"},
		{"one minute ago", 61 * time.Second, "1 minute ago"},
		{"several minutes ago", 5 * time.Minute, "5 minutes ago"},
		{"one hour ago", 61 * time.Minute, "1 hour ago"},
		{"several hours ago", 3 * time.Hour, "3 hours ago"},
		{"one day ago", 25 * time.Hour, "1 day ago"},
		{"several days ago", 3*24*time.Hour + 2*time.Hour, "3 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := timeAgo(time.Now().Add(-tt.ago)); got != tt.want {
				t.Errorf("timeAgo(%v ago) = %q, want %q", tt.ago, got, tt.want)
			}
		})
	}
}

func TestHighlightQuotesHappyPath(t *testing.T) {
	body := "the quick brown fox jumps over the lazy dog"
	citations := []model.Citation{
		{ID: "cit-1", VerbatimQuote: "quick brown", Polarity: "supports"},
		{ID: "cit-2", VerbatimQuote: "lazy dog", Polarity: "contradicts"},
	}
	want := `the <mark class="cite cite-supports" data-citation-id="cit-1">quick brown</mark> fox jumps over the <mark class="cite cite-contradicts" data-citation-id="cit-2">lazy dog</mark>`
	if got := string(highlightQuotes(body, citations)); got != want {
		t.Errorf("highlightQuotes() mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestHighlightQuotesOverlapResolvedLeftToRight(t *testing.T) {
	body := "the quick brown fox"
	citations := []model.Citation{
		{ID: "cit-1", VerbatimQuote: "quick brown", Polarity: "supports"},
		{ID: "cit-2", VerbatimQuote: "brown fox", Polarity: "contradicts"},
	}
	want := `the <mark class="cite cite-supports" data-citation-id="cit-1">quick brown</mark> fox`
	got := string(highlightQuotes(body, citations))
	if got != want {
		t.Errorf("highlightQuotes() mismatch:\n got: %s\nwant: %s", got, want)
	}
	if strings.Contains(got, "cit-2") {
		t.Errorf("later overlapping citation leaked into output: %s", got)
	}
}

func TestHighlightQuotesQuoteNotFound(t *testing.T) {
	body := "the quick brown fox"
	citations := []model.Citation{
		{ID: "cit-1", VerbatimQuote: "no such quote", Polarity: "supports"},
	}
	want := "the quick brown fox"
	if got := string(highlightQuotes(body, citations)); got != want {
		t.Errorf("highlightQuotes() = %q, want %q", got, want)
	}
}

func TestHighlightQuotesEscapesUntrustedBody(t *testing.T) {
	body := `<script>alert(1)</script> the quick fox`
	citations := []model.Citation{
		{ID: "cit-1", VerbatimQuote: "quick fox", Polarity: "supports"},
	}
	want := `&lt;script&gt;alert(1)&lt;/script&gt; the <mark class="cite cite-supports" data-citation-id="cit-1">quick fox</mark>`
	if got := string(highlightQuotes(body, citations)); got != want {
		t.Errorf("highlightQuotes() mismatch:\n got: %s\nwant: %s", got, want)
	}
}
