CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    accuracy REAL NOT NULL DEFAULT 1.0,
    contribution REAL NOT NULL DEFAULT 1.0,
    weight REAL NOT NULL DEFAULT 2.0,
    metadata TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE topics (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT,
    embedding BLOB,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE topic_exclusions (
    id TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    embedding BLOB NOT NULL,
    threshold REAL NOT NULL DEFAULT 0.3,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE claims (
    id TEXT PRIMARY KEY,
    proposition TEXT NOT NULL,
    embedding BLOB,
    groundedness REAL NOT NULL DEFAULT 0.0,
    effective_groundedness REAL NOT NULL DEFAULT 0.0,
    contestation REAL NOT NULL DEFAULT 0.0,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK(status IN ('active', 'contested', 'emerging', 'grounded', 'refuted', 'adjudicated')),
    adjudicated_value REAL,
    adjudicated_at DATETIME,
    adjudicated_by TEXT,
    adjudication_reasoning TEXT,
    parent_claim_id TEXT REFERENCES claims(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    computed_at DATETIME
);

CREATE TABLE assertions (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id),
    claim_id TEXT NOT NULL REFERENCES claims(id),
    stance TEXT NOT NULL CHECK(stance IN ('support', 'contest', 'refine')),
    confidence REAL NOT NULL CHECK(confidence >= 0.0 AND confidence <= 1.0),
    reasoning TEXT,
    sources TEXT,
    helpfulness REAL NOT NULL DEFAULT 0.0,
    refinement_claim_id TEXT REFERENCES claims(id),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(agent_id, claim_id)
);

CREATE TABLE assertion_history (
    id TEXT PRIMARY KEY,
    assertion_id TEXT NOT NULL REFERENCES assertions(id),
    agent_id TEXT NOT NULL,
    claim_id TEXT NOT NULL,
    stance TEXT NOT NULL,
    confidence REAL NOT NULL,
    reasoning TEXT,
    sources TEXT,
    replaced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE reviews (
    id TEXT PRIMARY KEY,
    reviewer_id TEXT NOT NULL REFERENCES agents(id),
    assertion_id TEXT NOT NULL REFERENCES assertions(id),
    helpfulness REAL NOT NULL CHECK(helpfulness >= 0.0 AND helpfulness <= 1.0),
    reasoning TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(reviewer_id, assertion_id)
);

CREATE TABLE dependencies (
    id TEXT PRIMARY KEY,
    claim_id TEXT NOT NULL REFERENCES claims(id),
    depends_on_id TEXT NOT NULL REFERENCES claims(id),
    strength REAL NOT NULL DEFAULT 1.0,
    reasoning TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(claim_id, depends_on_id)
);

CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id),
    token_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'agent' CHECK(role IN ('agent', 'admin')),
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE epochs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    accuracy_iterations INTEGER,
    contribution_iterations INTEGER,
    accuracy_delta REAL,
    contribution_delta REAL
);

CREATE INDEX idx_claims_status ON claims(status);
CREATE INDEX idx_claims_groundedness ON claims(groundedness);
CREATE INDEX idx_claims_contestation ON claims(contestation);
CREATE INDEX idx_claims_parent ON claims(parent_claim_id);
CREATE INDEX idx_assertions_agent ON assertions(agent_id);
CREATE INDEX idx_assertions_claim ON assertions(claim_id);
CREATE INDEX idx_reviews_reviewer ON reviews(reviewer_id);
CREATE INDEX idx_reviews_assertion ON reviews(assertion_id);
CREATE INDEX idx_dependencies_claim ON dependencies(claim_id);
CREATE INDEX idx_dependencies_depends_on ON dependencies(depends_on_id);
CREATE INDEX idx_agents_weight ON agents(weight);
CREATE INDEX idx_api_tokens_agent ON api_tokens(agent_id);
CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);
CREATE INDEX idx_assertion_history_assertion ON assertion_history(assertion_id);
