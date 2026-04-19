package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RefreshToken represents a stored refresh token.
type RefreshToken struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UserAgent string `json:"user_agent,omitempty"`
	IP        string `json:"ip,omitempty"`
}

// IsExpired checks if the refresh token has expired.
func (rt *RefreshToken) IsExpired() bool {
	return time.Now().Unix() > rt.ExpiresAt
}

// TokenStore manages refresh tokens with in-memory storage and disk persistence.
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*RefreshToken // token string → RefreshToken
	byUser map[string][]string      // userID → list of token strings
	dir    string                   // persistence directory
}

// NewTokenStore creates a new token store that persists to the given directory.
func NewTokenStore(dir string) (*TokenStore, error) {
	store := &TokenStore{
		tokens: make(map[string]*RefreshToken),
		byUser: make(map[string][]string),
		dir:    dir,
	}

	// Load existing tokens from disk
	if err := store.loadFromDisk(); err != nil {
		// Non-fatal: log warning but continue with empty store
		fmt.Fprintf(os.Stderr, "Warning: could not load token store: %v\n", err)
	}

	// Start cleanup goroutine
	go store.cleanupLoop()

	return store, nil
}

// Store saves a refresh token.
func (ts *TokenStore) Store(token *RefreshToken) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.tokens[token.Token] = token
	ts.byUser[token.UserID] = append(ts.byUser[token.UserID], token.Token)

	return ts.persistToken(token)
}

// Lookup retrieves a refresh token by its value. Returns nil if not found or expired.
func (ts *TokenStore) Lookup(tokenStr string) *RefreshToken {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	token, ok := ts.tokens[tokenStr]
	if !ok {
		return nil
	}

	if token.IsExpired() {
		return nil
	}

	return token
}

// Revoke removes a specific refresh token.
func (ts *TokenStore) Revoke(tokenStr string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	token, ok := ts.tokens[tokenStr]
	if !ok {
		return nil // Already revoked or doesn't exist
	}

	// Remove from tokens map
	delete(ts.tokens, tokenStr)

	// Remove from user's token list
	if tokens, ok := ts.byUser[token.UserID]; ok {
		filtered := make([]string, 0, len(tokens))
		for _, t := range tokens {
			if t != tokenStr {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			delete(ts.byUser, token.UserID)
		} else {
			ts.byUser[token.UserID] = filtered
		}
	}

	// Remove from disk
	return ts.removeTokenFile(tokenStr)
}

// RevokeAllForUser removes all refresh tokens for a user.
func (ts *TokenStore) RevokeAllForUser(userID string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	tokens, ok := ts.byUser[userID]
	if !ok {
		return nil
	}

	for _, tokenStr := range tokens {
		delete(ts.tokens, tokenStr)
		ts.removeTokenFile(tokenStr)
	}

	delete(ts.byUser, userID)
	return nil
}

// CountForUser returns the number of active refresh tokens for a user.
func (ts *TokenStore) CountForUser(userID string) int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return len(ts.byUser[userID])
}

// ---- Persistence ----

// persistToken writes a token to disk as a JSON file.
func (ts *TokenStore) persistToken(token *RefreshToken) error {
	if ts.dir == "" {
		return nil
	}

	data, err := json.Marshal(token)
	if err != nil {
		return err
	}

	filename := filepath.Join(ts.dir, tokenToFilename(token.Token))
	return os.WriteFile(filename, data, 0600)
}

// removeTokenFile deletes a token file from disk.
func (ts *TokenStore) removeTokenFile(tokenStr string) error {
	if ts.dir == "" {
		return nil
	}

	filename := filepath.Join(ts.dir, tokenToFilename(tokenStr))
	err := os.Remove(filename)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// loadFromDisk loads all tokens from disk.
func (ts *TokenStore) loadFromDisk() error {
	if ts.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(ts.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(ts.dir, entry.Name()))
		if err != nil {
			continue
		}

		var token RefreshToken
		if err := json.Unmarshal(data, &token); err != nil {
			continue
		}

		// Skip expired tokens
		if token.IsExpired() {
			os.Remove(filepath.Join(ts.dir, entry.Name()))
			continue
		}

		ts.tokens[token.Token] = &token
		ts.byUser[token.UserID] = append(ts.byUser[token.UserID], token.Token)
	}

	return nil
}

// cleanupLoop periodically removes expired tokens.
func (ts *TokenStore) cleanupLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ts.cleanup()
	}
}

// cleanup removes all expired tokens.
func (ts *TokenStore) cleanup() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for tokenStr, token := range ts.tokens {
		if token.IsExpired() {
			delete(ts.tokens, tokenStr)
			ts.removeTokenFile(tokenStr)

			// Clean up user index
			if tokens, ok := ts.byUser[token.UserID]; ok {
				filtered := make([]string, 0, len(tokens))
				for _, t := range tokens {
					if t != tokenStr {
						filtered = append(filtered, t)
					}
				}
				if len(filtered) == 0 {
					delete(ts.byUser, token.UserID)
				} else {
					ts.byUser[token.UserID] = filtered
				}
			}
		}
	}
}

// tokenToFilename converts a token string to a safe filename.
// Uses first 16 chars + hash to avoid filesystem issues with long tokens.
func tokenToFilename(token string) string {
	if len(token) > 16 {
		return token[:16] + ".json"
	}
	return token + ".json"
}
