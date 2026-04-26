package model

import "time"

// Agent is a research worker (extractor / auditor / both / observer / admin).
type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Reliability  float64   `json:"reliability"`
	Productivity float64   `json:"productivity"`
	Metadata     *string   `json:"metadata,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Topic struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	Embedding   []byte    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

type TopicExclusion struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Embedding   []byte    `json:"-"`
	Threshold   float64   `json:"threshold"`
	CreatedAt   time.Time `json:"created_at"`
}

// Source is a fetched, content-addressed body of evidence.
type Source struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	ContentHash string    `json:"content_hash"`
	BodyBlobID  string    `json:"body_blob_id"`
	FetchedAt   time.Time `json:"fetched_at"`
	Type        string    `json:"type"`
	Title       *string   `json:"title,omitempty"`
	Metadata    *string   `json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// SourceAnchor is the admin-curated prior on a source.
type SourceAnchor struct {
	SourceID    string    `json:"source_id"`
	Tier        int       `json:"tier"`
	Credibility float64   `json:"credibility"`
	SetBy       string    `json:"set_by"`
	Reasoning   *string   `json:"reasoning,omitempty"`
	SetAt       time.Time `json:"set_at"`
}

type SourceTag struct {
	SourceID string `json:"source_id"`
	Tag      string `json:"tag"`
}

// SourceCredibility is the per-epoch computed value.
type SourceCredibility struct {
	SourceID   string  `json:"source_id"`
	EpochID    int     `json:"epoch_id"`
	Value      float64 `json:"value"`
	Components *string `json:"components,omitempty"`
}

// SourceCitationEdge is a parsed reference from one source to another.
type SourceCitationEdge struct {
	FromSourceID string    `json:"from_source_id"`
	ToSourceID   string    `json:"to_source_id"`
	Locator      *string   `json:"locator,omitempty"`
	ExtractedAt  time.Time `json:"extracted_at"`
}

type Claim struct {
	ID                    string     `json:"id"`
	Proposition           string     `json:"proposition"`
	Embedding             []byte     `json:"-"`
	Groundedness          float64    `json:"groundedness"`
	EffectiveGroundedness float64    `json:"effective_groundedness"`
	Contestation          float64    `json:"contestation"`
	Status                string     `json:"status"`
	AdjudicatedValue      *float64   `json:"adjudicated_value,omitempty"`
	AdjudicatedAt         *time.Time `json:"adjudicated_at,omitempty"`
	AdjudicatedBy         *string    `json:"adjudicated_by,omitempty"`
	AdjudicationReasoning *string    `json:"adjudication_reasoning,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	ComputedAt            *time.Time `json:"computed_at,omitempty"`
}

// Citation is the link between a claim and a source via a verbatim quote.
type Citation struct {
	ID            string    `json:"id"`
	ClaimID       string    `json:"claim_id"`
	SourceID      string    `json:"source_id"`
	VerbatimQuote string    `json:"verbatim_quote"`
	Locator       *string   `json:"locator,omitempty"`
	Polarity      string    `json:"polarity"`
	Reasoning     *string   `json:"reasoning,omitempty"`
	ExtractorID   string    `json:"extractor_id"`
	AuditFactor   float64   `json:"audit_factor"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// Audit is an agent's verdict on someone else's citation.
type Audit struct {
	ID         string    `json:"id"`
	CitationID string    `json:"citation_id"`
	AuditorID  string    `json:"auditor_id"`
	Mechanical string    `json:"mechanical"`
	Semantic   string    `json:"semantic"`
	Verdict    string    `json:"verdict"`
	Reasoning  *string   `json:"reasoning,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Dependency struct {
	ID          string    `json:"id"`
	ClaimID     string    `json:"claim_id"`
	DependsOnID string    `json:"depends_on_id"`
	Strength    float64   `json:"strength"`
	Reasoning   *string   `json:"reasoning,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Lens is a saveable, forkable view of the knowledge graph under different
// source-credibility priors.
type Lens struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	OwnerID      *string   `json:"owner_id,omitempty"`
	ParentLensID *string   `json:"parent_lens_id,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Public       bool      `json:"public"`
	CreatedAt    time.Time `json:"created_at"`
}

// LensOverride is a per-source credibility override for a lens.
//
//	mode = "absolute"   -> set credibility := value
//	mode = "multiplier" -> credibility *= value
//	mode = "exclude"    -> credibility := 0
type LensOverride struct {
	LensID   string  `json:"lens_id"`
	SourceID string  `json:"source_id"`
	Mode     string  `json:"mode"`
	Value    float64 `json:"value"`
}

// LensTagOverride applies a multiplier to all sources carrying a tag.
type LensTagOverride struct {
	LensID     string  `json:"lens_id"`
	Tag        string  `json:"tag"`
	Multiplier float64 `json:"multiplier"`
}

type APIToken struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	TokenHash string     `json:"-"`
	Role      string     `json:"role"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type Epoch struct {
	ID               int        `json:"id"`
	StartedAt        time.Time  `json:"started_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	SourceIterations *int       `json:"source_iterations,omitempty"`
	AgentIterations  *int       `json:"agent_iterations,omitempty"`
	SourceDelta      *float64   `json:"source_delta,omitempty"`
	AgentDelta       *float64   `json:"agent_delta,omitempty"`
}
