package model

import "time"

type Agent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Accuracy     float64  `json:"accuracy"`
	Contribution float64  `json:"contribution"`
	Weight       float64  `json:"weight"`
	Metadata     *string  `json:"metadata,omitempty"`
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

type Claim struct {
	ID                     string     `json:"id"`
	Proposition            string     `json:"proposition"`
	Embedding              []byte     `json:"-"`
	Groundedness           float64    `json:"groundedness"`
	EffectiveGroundedness  float64    `json:"effective_groundedness"`
	Contestation           float64    `json:"contestation"`
	Status                 string     `json:"status"`
	AdjudicatedValue       *float64   `json:"adjudicated_value,omitempty"`
	AdjudicatedAt          *time.Time `json:"adjudicated_at,omitempty"`
	AdjudicatedBy          *string    `json:"adjudicated_by,omitempty"`
	AdjudicationReasoning  *string    `json:"adjudication_reasoning,omitempty"`
	ParentClaimID          *string    `json:"parent_claim_id,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	ComputedAt             *time.Time `json:"computed_at,omitempty"`
}

type Assertion struct {
	ID                string    `json:"id"`
	AgentID           string    `json:"agent_id"`
	ClaimID           string    `json:"claim_id"`
	Stance            string    `json:"stance"`
	Confidence        float64   `json:"confidence"`
	Reasoning         *string   `json:"reasoning,omitempty"`
	Sources           *string   `json:"sources,omitempty"`
	Helpfulness       float64   `json:"helpfulness"`
	RefinementClaimID *string   `json:"refinement_claim_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type AssertionHistory struct {
	ID          string    `json:"id"`
	AssertionID string    `json:"assertion_id"`
	AgentID     string    `json:"agent_id"`
	ClaimID     string    `json:"claim_id"`
	Stance      string    `json:"stance"`
	Confidence  float64   `json:"confidence"`
	Reasoning   *string   `json:"reasoning,omitempty"`
	Sources     *string   `json:"sources,omitempty"`
	ReplacedAt  time.Time `json:"replaced_at"`
}

type Review struct {
	ID          string    `json:"id"`
	ReviewerID  string    `json:"reviewer_id"`
	AssertionID string    `json:"assertion_id"`
	Helpfulness float64   `json:"helpfulness"`
	Reasoning   *string   `json:"reasoning,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Dependency struct {
	ID          string    `json:"id"`
	ClaimID     string    `json:"claim_id"`
	DependsOnID string    `json:"depends_on_id"`
	Strength    float64   `json:"strength"`
	Reasoning   *string   `json:"reasoning,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
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
	ID                     int        `json:"id"`
	StartedAt              time.Time  `json:"started_at"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
	AccuracyIterations     *int       `json:"accuracy_iterations,omitempty"`
	ContributionIterations *int       `json:"contribution_iterations,omitempty"`
	AccuracyDelta          *float64   `json:"accuracy_delta,omitempty"`
	ContributionDelta      *float64   `json:"contribution_delta,omitempty"`
}
