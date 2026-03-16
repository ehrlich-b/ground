package api

import (
	"time"

	"github.com/ehrlich-b/ground/internal/db"
	"github.com/ehrlich-b/ground/internal/model"
)

// IssueToken creates a JWT and stores its hash in the database.
// Returns the raw JWT string.
func IssueToken(store *db.Store, jwtSecret []byte, agentID, role string) (string, error) {
	tokenStr, err := createJWT(jwtSecret, agentID, role)
	if err != nil {
		return "", err
	}

	tok := &model.APIToken{
		ID:        generateID(),
		AgentID:   agentID,
		TokenHash: hashToken(tokenStr),
		Role:      role,
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
	}
	if err := store.CreateAPIToken(tok); err != nil {
		return "", err
	}

	return tokenStr, nil
}
