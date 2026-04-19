package auth

import (
	"time"

	"github.com/ayushkunwarsingh/forge/utils"
)

// SessionManager handles creation of access tokens and refresh tokens.
type SessionManager struct {
	jwt           *JWTManager
	tokenStore    *TokenStore
	refreshExpiry time.Duration
}

// NewSessionManager creates a new session manager.
func NewSessionManager(jwt *JWTManager, tokenStore *TokenStore, refreshExpiry time.Duration) *SessionManager {
	return &SessionManager{
		jwt:           jwt,
		tokenStore:    tokenStore,
		refreshExpiry: refreshExpiry,
	}
}

// TokenPair represents an access token + refresh token pair.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"` // seconds until access token expires
}

// CreateSession generates a new access token + refresh token pair for a user.
func (sm *SessionManager) CreateSession(user *User, userAgent, ip string) (*TokenPair, error) {
	// Create JWT access token
	claims := &JWTClaims{
		Sub:           user.UID,
		Email:         user.Email,
		EmailVerified: user.EmailVerified,
		Name:          user.DisplayName,
		Picture:       user.PhotoURL,
		Admin:         user.Admin,
		Custom:        user.CustomClaims,
	}

	accessToken, err := sm.jwt.GenerateToken(claims)
	if err != nil {
		return nil, err
	}

	// Generate refresh token (256-bit random)
	refreshTokenStr, err := utils.RandomHex(32) // 64 hex chars
	if err != nil {
		return nil, err
	}

	// Store refresh token
	refreshToken := &RefreshToken{
		Token:     refreshTokenStr,
		UserID:    user.UID,
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(sm.refreshExpiry).Unix(),
		UserAgent: userAgent,
		IP:        ip,
	}

	if err := sm.tokenStore.Store(refreshToken); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
		TokenType:    "Bearer",
		ExpiresIn:    int64(sm.jwt.expiry.Seconds()),
	}, nil
}

// RefreshSession exchanges a refresh token for a new token pair.
// The old refresh token is revoked and a new one is issued (rotation).
func (sm *SessionManager) RefreshSession(refreshTokenStr string, getUser func(uid string) *User, userAgent, ip string) (*TokenPair, error) {
	// Look up the refresh token
	token := sm.tokenStore.Lookup(refreshTokenStr)
	if token == nil {
		return nil, ErrInvalidToken
	}

	// Get the user
	user := getUser(token.UserID)
	if user == nil {
		// User was deleted — revoke the token
		sm.tokenStore.Revoke(refreshTokenStr)
		return nil, ErrInvalidToken
	}

	if user.Disabled {
		sm.tokenStore.Revoke(refreshTokenStr)
		return nil, ErrInvalidToken
	}

	// Revoke the old refresh token (rotation)
	sm.tokenStore.Revoke(refreshTokenStr)

	// Create new session
	return sm.CreateSession(user, userAgent, ip)
}

// RevokeSession revokes a specific refresh token (signout).
func (sm *SessionManager) RevokeSession(refreshTokenStr string) error {
	return sm.tokenStore.Revoke(refreshTokenStr)
}

// RevokeAllSessions revokes all refresh tokens for a user (force signout everywhere).
func (sm *SessionManager) RevokeAllSessions(userID string) error {
	return sm.tokenStore.RevokeAllForUser(userID)
}
