package sources

import (
	"reflect"
	"testing"
)

func TestGuessSourceType(t *testing.T) {
	cases := []struct {
		name        string
		rawURL      string
		contentType string
		want        string
	}{
		{
			name:   "arxiv by https prefix",
			rawURL: "https://arxiv.org/abs/2401.00001",
			want:   "arxiv",
		},
		{
			name:   "arxiv by http prefix",
			rawURL: "http://arxiv.org/abs/2401.00001",
			want:   "arxiv",
		},
		{
			name:   "pubmed by ncbi host",
			rawURL: "https://www.ncbi.nlm.nih.gov/pubmed/12345678",
			want:   "pubmed",
		},
		{
			name:   "pdf by url suffix",
			rawURL: "https://example.com/whitepaper.pdf",
			want:   "pdf",
		},
		{
			name:        "pdf by content type",
			rawURL:      "https://example.com/report",
			contentType: "application/pdf",
			want:        "pdf",
		},
		{
			name:        "plain by content type",
			rawURL:      "https://example.com/notes.txt",
			contentType: "text/plain; charset=utf-8",
			want:        "plain",
		},
		{
			name:        "default html",
			rawURL:      "https://example.com/article",
			contentType: "text/html; charset=utf-8",
			want:        "html",
		},
		{
			name:   "precedence pubmed beats pdf url suffix",
			rawURL: "https://www.ncbi.nlm.nih.gov/pmc/articles/PMC1234567.pdf",
			want:   "pubmed",
		},
		{
			name:        "precedence pubmed beats pdf content type",
			rawURL:      "https://www.ncbi.nlm.nih.gov/pmc/articles/PMC7654321",
			contentType: "application/pdf",
			want:        "pubmed",
		},
		{
			name:        "precedence arxiv beats pdf content type",
			rawURL:      "https://arxiv.org/pdf/2401.00001",
			contentType: "application/pdf",
			want:        "arxiv",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := guessSourceType(tc.rawURL, tc.contentType); got != tc.want {
				t.Errorf("guessSourceType(%q, %q) = %q, want %q", tc.rawURL, tc.contentType, got, tc.want)
			}
		})
	}
}

func TestAutoTagsTypeSwitch(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		srcType string
		want    []string
	}{
		{
			name:    "arxiv type tags",
			rawURL:  "https://arxiv.org/abs/2401.00001",
			srcType: "arxiv",
			want:    []string{"preprint", "physics", "cs"},
		},
		{
			name:    "pubmed type tags",
			rawURL:  "https://example.com/doc",
			srcType: "pubmed",
			want:    []string{"peer-reviewed", "biomed"},
		},
		{
			name:    "pdf type tags",
			rawURL:  "https://example.com/doc.pdf",
			srcType: "pdf",
			want:    []string{"pdf"},
		},
		{
			name:    "unmatched type produces empty non-nil slice",
			rawURL:  "https://example.com/doc",
			srcType: "html",
			want:    []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := autoTags(tc.rawURL, tc.srcType)
			if got == nil {
				t.Fatalf("autoTags(%q, %q) returned nil, want non-nil slice", tc.rawURL, tc.srcType)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("autoTags(%q, %q) = %v, want %v", tc.rawURL, tc.srcType, got, tc.want)
			}
		})
	}
}

func TestAutoTagsDomainSwitch(t *testing.T) {
	cases := []struct {
		name   string
		rawURL string
		want   []string
	}{
		{
			name:   "gov host suffix",
			rawURL: "https://www.nih.gov/research/report",
			want:   []string{"government"},
		},
		{
			name:   "wikipedia",
			rawURL: "https://en.wikipedia.org/wiki/Groundedness",
			want:   []string{"wiki"},
		},
		{
			name:   "tier-1 journal nature",
			rawURL: "https://www.nature.com/articles/s41586-024-00001-x",
			want:   []string{"peer-reviewed", "tier-1-journal"},
		},
		{
			name:   "tier-1 journal science",
			rawURL: "https://www.science.org/doi/10.1126/science.abc1234",
			want:   []string{"peer-reviewed", "tier-1-journal"},
		},
		{
			name:   "news ap",
			rawURL: "https://ap.org/hub/politics",
			want:   []string{"news"},
		},
		{
			name:   "news reuters",
			rawURL: "https://www.reuters.com/world/europe/story",
			want:   []string{"news"},
		},
		{
			name:   "blog substack",
			rawURL: "https://ground.substack.com/p/hello",
			want:   []string{"blog"},
		},
		{
			name:   "blog medium",
			rawURL: "https://medium.com/@ground/on-grounding",
			want:   []string{"blog"},
		},
		{
			name:   "unmatched domain produces empty non-nil slice",
			rawURL: "https://example.com/article/1",
			want:   []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := autoTags(tc.rawURL, "plain")
			if got == nil {
				t.Fatalf("autoTags(%q, %q) returned nil, want non-nil slice", tc.rawURL, "plain")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("autoTags(%q, %q) = %v, want %v", tc.rawURL, "plain", got, tc.want)
			}
		})
	}
}

func TestAutoTagsBothSwitchesCombine(t *testing.T) {
	got := autoTags("https://www.nih.gov/static/updated-guidance.pdf", "pdf")
	want := []string{"pdf", "government"}
	if got == nil {
		t.Fatal("autoTags(...) returned nil, want non-nil slice")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("autoTags(.gov URL with srcType pdf) = %v, want %v", got, want)
	}
}

func TestParseHost(t *testing.T) {
	cases := []struct {
		name   string
		rawURL string
		want   string
	}{
		{
			name:   "normal url keeps port",
			rawURL: "https://example.com:8080/path?q=1",
			want:   "example.com:8080",
		},
		{
			name:   "bare domain without scheme parses as path",
			rawURL: "example.com/path",
			want:   "",
		},
		{
			name:   "host is lowercased",
			rawURL: "https://WWW.EXAMPLE.COM/Path",
			want:   "www.example.com",
		},
		{
			name:   "malformed url returns empty not panic",
			rawURL: "://not-a-url",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseHost(tc.rawURL); got != tc.want {
				t.Errorf("parseHost(%q) = %q, want %q", tc.rawURL, got, tc.want)
			}
		})
	}
}
