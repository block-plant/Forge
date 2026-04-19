package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

// JWT errors
var (
	ErrInvalidToken    = errors.New("jwt: invalid token")
	ErrTokenExpired    = errors.New("jwt: token has expired")
	ErrInvalidSignature = errors.New("jwt: invalid signature")
	ErrMalformedToken  = errors.New("jwt: malformed token")
	ErrUnsupportedAlg  = errors.New("jwt: unsupported algorithm")
)

// JWTHeader is the JWT header.
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid,omitempty"`
}

// JWTClaims represents the payload of a JWT token.
type JWTClaims struct {
	// Standard claims
	Sub string `json:"sub"`           // Subject (user ID)
	Iss string `json:"iss,omitempty"` // Issuer
	Aud string `json:"aud,omitempty"` // Audience
	Exp int64  `json:"exp"`           // Expiration time (Unix)
	Iat int64  `json:"iat"`           // Issued at (Unix)
	Nbf int64  `json:"nbf,omitempty"` // Not before (Unix)
	Jti string `json:"jti,omitempty"` // JWT ID

	// Forge custom claims
	Email         string                 `json:"email,omitempty"`
	EmailVerified bool                   `json:"email_verified,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Picture       string                 `json:"picture,omitempty"`
	Admin         bool                   `json:"admin,omitempty"`
	Custom        map[string]interface{} `json:"custom,omitempty"`
}

// IsExpired checks if the token has expired.
func (c *JWTClaims) IsExpired() bool {
	return time.Now().Unix() > c.Exp
}

// JWTManager handles JWT creation and verification using RS256.
type JWTManager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	issuer     string
	expiry     time.Duration
}

// NewJWTManager creates a new JWT manager.
// It loads or generates RSA keys from the given paths.
func NewJWTManager(privateKeyPath, publicKeyPath, issuer string, expiry time.Duration, keySize int) (*JWTManager, error) {
	var privateKey *rsa.PrivateKey

	// Try to load existing keys
	privData, err := os.ReadFile(privateKeyPath)
	if err == nil {
		privateKey, err = parsePrivateKey(privData)
		if err != nil {
			return nil, fmt.Errorf("jwt: failed to parse private key: %w", err)
		}
	} else if os.IsNotExist(err) {
		// Generate new RSA key pair
		privateKey, err = rsa.GenerateKey(rand.Reader, keySize)
		if err != nil {
			return nil, fmt.Errorf("jwt: failed to generate RSA key: %w", err)
		}

		// Save private key
		if err := savePrivateKey(privateKey, privateKeyPath); err != nil {
			return nil, fmt.Errorf("jwt: failed to save private key: %w", err)
		}

		// Save public key
		if err := savePublicKey(&privateKey.PublicKey, publicKeyPath); err != nil {
			return nil, fmt.Errorf("jwt: failed to save public key: %w", err)
		}
	} else {
		return nil, fmt.Errorf("jwt: failed to read private key file: %w", err)
	}

	return &JWTManager{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		issuer:     issuer,
		expiry:     expiry,
	}, nil
}

// GenerateToken creates a new signed JWT for the given claims.
func (m *JWTManager) GenerateToken(claims *JWTClaims) (string, error) {
	now := time.Now()

	// Set standard fields
	claims.Iss = m.issuer
	claims.Iat = now.Unix()
	if claims.Exp == 0 {
		claims.Exp = now.Add(m.expiry).Unix()
	}

	// Build the header
	header := JWTHeader{
		Alg: "RS256",
		Typ: "JWT",
	}

	// Encode header
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("jwt: failed to marshal header: %w", err)
	}
	headerEncoded := base64URLEncode(headerJSON)

	// Encode payload
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("jwt: failed to marshal claims: %w", err)
	}
	payloadEncoded := base64URLEncode(payloadJSON)

	// Create signing input
	signingInput := headerEncoded + "." + payloadEncoded

	// Sign with RSA-SHA256
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, m.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("jwt: failed to sign token: %w", err)
	}
	signatureEncoded := base64URLEncode(signature)

	return signingInput + "." + signatureEncoded, nil
}

// VerifyToken verifies a JWT token and returns its claims.
func (m *JWTManager) VerifyToken(tokenString string) (*JWTClaims, error) {
	// Split into 3 parts
	parts := strings.SplitN(tokenString, ".", 3)
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}

	headerEncoded := parts[0]
	payloadEncoded := parts[1]
	signatureEncoded := parts[2]

	// Decode and verify header
	headerJSON, err := base64URLDecode(headerEncoded)
	if err != nil {
		return nil, fmt.Errorf("jwt: failed to decode header: %w", err)
	}

	var header JWTHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("jwt: failed to parse header: %w", err)
	}

	if header.Alg != "RS256" {
		return nil, ErrUnsupportedAlg
	}

	// Verify signature
	signingInput := headerEncoded + "." + payloadEncoded
	signature, err := base64URLDecode(signatureEncoded)
	if err != nil {
		return nil, fmt.Errorf("jwt: failed to decode signature: %w", err)
	}

	hash := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(m.publicKey, crypto.SHA256, hash[:], signature); err != nil {
		return nil, ErrInvalidSignature
	}

	// Decode payload
	payloadJSON, err := base64URLDecode(payloadEncoded)
	if err != nil {
		return nil, fmt.Errorf("jwt: failed to decode payload: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("jwt: failed to parse claims: %w", err)
	}

	// Check expiration
	if claims.IsExpired() {
		return nil, ErrTokenExpired
	}

	return &claims, nil
}

// PublicKeyJWKS returns the public key in JWKS format for the /.well-known/jwks.json endpoint.
func (m *JWTManager) PublicKeyJWKS() map[string]interface{} {
	n := m.publicKey.N
	e := m.publicKey.E

	// Encode modulus as base64url
	nBytes := n.Bytes()
	nEncoded := base64URLEncode(nBytes)

	// Encode exponent as base64url
	eBytes := big.NewInt(int64(e)).Bytes()
	eEncoded := base64URLEncode(eBytes)

	return map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   nEncoded,
				"e":   eEncoded,
			},
		},
	}
}

// ---- Base64 URL Encoding (no padding) ----

// base64URLEncode encodes data using base64url without padding (RFC 4648 §5).
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// base64URLDecode decodes base64url data without padding.
func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// ---- RSA Key Persistence ----

// parsePrivateKey parses a PEM-encoded RSA private key.
func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}

	// Try PKCS#8 first, then PKCS#1
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not RSA")
	}
	return rsaKey, nil
}

// savePrivateKey writes an RSA private key as a PEM file.
func savePrivateKey(key *rsa.PrivateKey, path string) error {
	derBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: derBytes,
	}

	return os.WriteFile(path, pem.EncodeToMemory(block), 0600)
}

// savePublicKey writes an RSA public key as a PEM file.
func savePublicKey(key *rsa.PublicKey, path string) error {
	derBytes, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return err
	}

	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derBytes,
	}

	return os.WriteFile(path, pem.EncodeToMemory(block), 0644)
}
