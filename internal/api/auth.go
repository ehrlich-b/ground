package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}

func createJWT(secret []byte, agentID, role string) (string, error) {
	claims := Claims{
		AgentID: agentID,
		Role:    role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(90 * 24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

func parseJWT(secret []byte, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

// hashToken returns the SHA-256 hex hash of a raw token string.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// generateRawToken creates a random 32-byte hex token.
func generateRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// authMiddleware extracts and validates JWT from Authorization header.
// Sets agentID and role in request context.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing Authorization header", nil)
			return
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		if tokenStr == auth {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid Authorization format, use Bearer <token>", nil)
			return
		}

		claims, err := parseJWT(s.jwtSecret, tokenStr)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token", nil)
			return
		}

		// Check token is not revoked
		h := hashToken(tokenStr)
		tok, err := s.store.GetAPITokenByHash(h)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token not found", nil)
			return
		}
		if tok.RevokedAt != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token has been revoked", nil)
			return
		}

		ctx := withAgentID(r.Context(), claims.AgentID)
		ctx = withRole(ctx, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdmin wraps a handler to require admin role.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if getRole(r.Context()) != "admin" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "admin access required", nil)
			return
		}
		next(w, r)
	}
}
