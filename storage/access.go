package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/ayushkunwarsingh/forge/logger"
)

// AccessController manages access control for stored files.
// It generates and verifies time-limited signed URLs using HMAC-SHA256.
type AccessController struct {
	secretKey []byte
	log       *logger.Logger
}

// SignedURLParams contains the parameters for a signed URL.
type SignedURLParams struct {
	// Path is the file path to sign.
	Path string `json:"path"`
	// Expiry is how long the signed URL should be valid.
	Expiry time.Duration `json:"expiry"`
	// Method restricts the URL to a specific HTTP method (GET, PUT, etc.).
	// Empty means GET only.
	Method string `json:"method,omitempty"`
}

// SignedURL represents a generated signed URL with its components.
type SignedURL struct {
	// URL is the full signed URL path with query parameters.
	URL string `json:"url"`
	// Token is the HMAC signature.
	Token string `json:"token"`
	// ExpiresAt is when the signed URL expires.
	ExpiresAt time.Time `json:"expires_at"`
}

// NewAccessController creates a new access controller with the given secret key.
func NewAccessController(secretKey []byte, log *logger.Logger) *AccessController {
	return &AccessController{
		secretKey: secretKey,
		log:       log,
	}
}

// GenerateSignedURL creates a time-limited signed URL for the given file path.
// The signed URL grants temporary public access to a file.
func (ac *AccessController) GenerateSignedURL(params SignedURLParams) (*SignedURL, error) {
	if params.Path == "" {
		return nil, fmt.Errorf("access: path is required")
	}

	if params.Expiry <= 0 {
		params.Expiry = 1 * time.Hour // Default 1 hour
	}

	if params.Method == "" {
		params.Method = "GET"
	}

	expiresAt := time.Now().UTC().Add(params.Expiry)
	expiresUnix := expiresAt.Unix()

	// Create the string to sign: method:path:expiry
	signingString := fmt.Sprintf("%s:%s:%d", params.Method, params.Path, expiresUnix)

	// Compute HMAC-SHA256
	token := ac.computeHMAC(signingString)

	// Build the URL
	url := fmt.Sprintf("/storage/object/%s?token=%s&expires=%d&method=%s",
		params.Path, token, expiresUnix, params.Method)

	return &SignedURL{
		URL:       url,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

// VerifySignedURL verifies that a signed URL token is valid and not expired.
func (ac *AccessController) VerifySignedURL(path, token, expiresStr, method string) error {
	if method == "" {
		method = "GET"
	}

	// Parse expiry
	expiresUnix, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil {
		return fmt.Errorf("access: invalid expiry format")
	}

	// Check expiration
	expiresAt := time.Unix(expiresUnix, 0)
	if time.Now().UTC().After(expiresAt) {
		return fmt.Errorf("access: signed URL has expired")
	}

	// Verify HMAC
	signingString := fmt.Sprintf("%s:%s:%d", method, path, expiresUnix)
	expectedToken := ac.computeHMAC(signingString)

	if !hmac.Equal([]byte(token), []byte(expectedToken)) {
		return fmt.Errorf("access: invalid signed URL token")
	}

	return nil
}

// computeHMAC computes HMAC-SHA256 and returns hex-encoded string.
func (ac *AccessController) computeHMAC(message string) string {
	mac := hmac.New(sha256.New, ac.secretKey)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
