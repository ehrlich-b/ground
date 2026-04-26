-- Ground v2 schema. Single migration, fresh start.

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'both'
        CHECK(role IN ('extractor', 'auditor', 'both', 'observer', 'admin')),
    reliability REAL NOT NULL DEFAULT 0.5,
    productivity REAL NOT NULL DEFAULT 0.0,
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

CREATE TABLE sources (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    body_blob_id TEXT NOT NULL,
    fetched_at DATETIME NOT NULL,
    type TEXT NOT NULL DEFAULT 'html'
        CHECK(type IN ('html', 'pdf', 'arxiv', 'pubmed', 'plain', 'unverifiable')),
    title TEXT,
    metadata TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(content_hash)
);

CREATE INDEX idx_sources_url ON sources(url);
CREATE INDEX idx_sources_content_hash ON sources(content_hash);

CREATE TABLE source_anchors (
    source_id TEXT PRIMARY KEY REFERENCES sources(id),
    tier INTEGER NOT NULL CHECK(tier BETWEEN 1 AND 4),
    credibility REAL NOT NULL CHECK(credibility >= 0.0 AND credibility <= 1.0),
    set_by TEXT NOT NULL,
    reasoning TEXT,
    set_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE source_tags (
    source_id TEXT NOT NULL REFERENCES sources(id),
    tag TEXT NOT NULL,
    PRIMARY KEY (source_id, tag)
);

CREATE INDEX idx_source_tags_tag ON source_tags(tag);

CREATE TABLE source_credibility (
    source_id TEXT NOT NULL REFERENCES sources(id),
    epoch_id INTEGER NOT NULL REFERENCES epochs(id),
    value REAL NOT NULL,
    components TEXT,
    PRIMARY KEY (source_id, epoch_id)
);

CREATE TABLE source_citation_edges (
    from_source_id TEXT NOT NULL REFERENCES sources(id),
    to_source_id TEXT NOT NULL REFERENCES sources(id),
    locator TEXT,
    extracted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (from_source_id, to_source_id)
);

CREATE INDEX idx_source_edges_to ON source_citation_edges(to_source_id);

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
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    computed_at DATETIME
);

CREATE INDEX idx_claims_status ON claims(status);
CREATE INDEX idx_claims_groundedness ON claims(groundedness);
CREATE INDEX idx_claims_contestation ON claims(contestation);

CREATE TABLE citations (
    id TEXT PRIMARY KEY,
    claim_id TEXT NOT NULL REFERENCES claims(id),
    source_id TEXT NOT NULL REFERENCES sources(id),
    verbatim_quote TEXT NOT NULL,
    locator TEXT,
    polarity TEXT NOT NULL CHECK(polarity IN ('supports', 'contradicts', 'qualifies')),
    reasoning TEXT,
    extractor_id TEXT NOT NULL REFERENCES agents(id),
    audit_factor REAL NOT NULL DEFAULT 1.0,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK(status IN ('active', 'rejected', 'drift_invalid')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_citations_claim ON citations(claim_id);
CREATE INDEX idx_citations_source ON citations(source_id);
CREATE INDEX idx_citations_extractor ON citations(extractor_id);
CREATE INDEX idx_citations_claim_polarity ON citations(claim_id, polarity);

CREATE TABLE audits (
    id TEXT PRIMARY KEY,
    citation_id TEXT NOT NULL REFERENCES citations(id),
    auditor_id TEXT NOT NULL REFERENCES agents(id),
    mechanical TEXT NOT NULL CHECK(mechanical IN ('pass', 'fail')),
    semantic TEXT NOT NULL CHECK(semantic IN ('confirm', 'misquote', 'out_of_context', 'weak', 'broken_link')),
    verdict TEXT NOT NULL CHECK(verdict IN ('uphold', 'reject')),
    reasoning TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(citation_id, auditor_id)
);

CREATE INDEX idx_audits_citation ON audits(citation_id);
CREATE INDEX idx_audits_auditor ON audits(auditor_id);

CREATE TABLE dependencies (
    id TEXT PRIMARY KEY,
    claim_id TEXT NOT NULL REFERENCES claims(id),
    depends_on_id TEXT NOT NULL REFERENCES claims(id),
    strength REAL NOT NULL DEFAULT 1.0,
    reasoning TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(claim_id, depends_on_id)
);

CREATE INDEX idx_dependencies_claim ON dependencies(claim_id);
CREATE INDEX idx_dependencies_depends_on ON dependencies(depends_on_id);

CREATE TABLE lenses (
    id TEXT PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    owner_id TEXT REFERENCES agents(id),
    parent_lens_id TEXT REFERENCES lenses(id),
    description TEXT,
    public INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_lenses_owner ON lenses(owner_id);

CREATE TABLE lens_overrides (
    lens_id TEXT NOT NULL REFERENCES lenses(id),
    source_id TEXT NOT NULL REFERENCES sources(id),
    mode TEXT NOT NULL CHECK(mode IN ('absolute', 'multiplier', 'exclude')),
    value REAL NOT NULL DEFAULT 0.0,
    PRIMARY KEY (lens_id, source_id)
);

CREATE TABLE lens_tag_overrides (
    lens_id TEXT NOT NULL REFERENCES lenses(id),
    tag TEXT NOT NULL,
    multiplier REAL NOT NULL DEFAULT 1.0,
    PRIMARY KEY (lens_id, tag)
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

CREATE INDEX idx_api_tokens_agent ON api_tokens(agent_id);
CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash);

CREATE TABLE epochs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    source_iterations INTEGER,
    agent_iterations INTEGER,
    source_delta REAL,
    agent_delta REAL
);
