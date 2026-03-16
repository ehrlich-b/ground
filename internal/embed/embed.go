package embed

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
)

// Embedder generates embedding vectors from text.
type Embedder interface {
	Embed(text string) ([]float32, error)
}

// OpenAI implements Embedder using the OpenAI API.
type OpenAI struct {
	apiKey string
	model  string
	client *http.Client
}

func NewOpenAI() (*OpenAI, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}
	return &OpenAI{
		apiKey: key,
		model:  "text-embedding-3-small",
		client: http.DefaultClient,
	}, nil
}

type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *OpenAI) Embed(text string) ([]float32, error) {
	body, err := json.Marshal(embeddingRequest{Input: text, Model: o.model})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result embeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai error: %s", result.Error.Message)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

// --- Vector serialization ---

// MarshalVector serializes a float32 slice to bytes (little-endian).
func MarshalVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// UnmarshalVector deserializes bytes to a float32 slice.
func UnmarshalVector(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	n := len(b) / 4
	v := make([]float32, n)
	for i := range n {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// --- Similarity ---

// CosineSimilarity returns the cosine similarity between two vectors.
// Returns 0 if either vector is zero-length or has zero magnitude.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// CosineDistance returns 1 - cosine similarity.
func CosineDistance(a, b []float32) float64 {
	return 1 - CosineSimilarity(a, b)
}

// --- Checks ---

// TopicMatch represents a topic's proximity to a query embedding.
type TopicMatch struct {
	TopicID    string
	Slug       string
	Title      string
	Similarity float64
}

// FindNearestTopics returns topics sorted by cosine similarity to the query embedding.
// Only topics with embeddings are considered.
func FindNearestTopics(query []float32, topics []TopicWithEmbedding, limit int) []TopicMatch {
	var matches []TopicMatch
	for _, t := range topics {
		vec := UnmarshalVector(t.Embedding)
		if len(vec) == 0 {
			continue
		}
		sim := CosineSimilarity(query, vec)
		matches = append(matches, TopicMatch{
			TopicID:    t.ID,
			Slug:       t.Slug,
			Title:      t.Title,
			Similarity: sim,
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// TopicWithEmbedding is a lightweight struct for topic proximity checks.
type TopicWithEmbedding struct {
	ID        string
	Slug      string
	Title     string
	Embedding []byte
}

// ExclusionAnchor is a lightweight struct for exclusion checks.
type ExclusionAnchor struct {
	ID          string
	Description string
	Embedding   []byte
	Threshold   float64
}

// CheckExclusions returns the first exclusion anchor that the query embedding
// is within threshold distance of, or nil if none match.
func CheckExclusions(query []float32, anchors []ExclusionAnchor) *ExclusionAnchor {
	for i := range anchors {
		vec := UnmarshalVector(anchors[i].Embedding)
		if len(vec) == 0 {
			continue
		}
		dist := CosineDistance(query, vec)
		if dist <= anchors[i].Threshold {
			return &anchors[i]
		}
	}
	return nil
}

// DuplicateMatch represents a similar existing claim.
type DuplicateMatch struct {
	ClaimID     string
	Proposition string
	Similarity  float64
}

// ClaimWithEmbedding is a lightweight struct for duplicate detection.
type ClaimWithEmbedding struct {
	ID          string
	Proposition string
	Embedding   []byte
}

// FindDuplicates returns claims with cosine similarity above threshold.
// Sorted by similarity descending.
func FindDuplicates(query []float32, claims []ClaimWithEmbedding, threshold float64) []DuplicateMatch {
	var matches []DuplicateMatch
	for _, c := range claims {
		vec := UnmarshalVector(c.Embedding)
		if len(vec) == 0 {
			continue
		}
		sim := CosineSimilarity(query, vec)
		if sim >= threshold {
			matches = append(matches, DuplicateMatch{
				ClaimID:     c.ID,
				Proposition: c.Proposition,
				Similarity:  sim,
			})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Similarity > matches[j].Similarity
	})
	return matches
}
