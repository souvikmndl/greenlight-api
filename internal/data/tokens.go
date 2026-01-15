package data

import (
	"crypto/rand"
	"crypto/sha256"
	"time"
)

const (
	// ScopeActivation to activate new user
	ScopeActivation = "activation"
)

// Token struct holds data for an individual token, including plaintext and hashed version
type Token struct {
	Plaintext string
	Hash      []byte
	UserID    int64
	Expiry    time.Time
	Scope     string
}

// generateToken creates the plaintext token, hash of token, expiry and scope
func generateToken(userID int64, ttl time.Duration, scope string) *Token {
	token := &Token{
		Plaintext: rand.Text(),
		UserID:    userID,
		Expiry:    time.Now().Add(ttl),
		Scope:     scope,
	}

	hash := sha256.Sum256([]byte(token.Plaintext))
	token.Hash = hash[:]

	return token
}
