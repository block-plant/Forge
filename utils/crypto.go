package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// RandomBytes generates n cryptographically secure random bytes.
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return b, nil
}

// RandomHex generates a random hex string of the given byte length.
// The resulting string will be 2*n characters long.
func RandomHex(n int) (string, error) {
	b, err := RandomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SHA256Hash computes the SHA-256 hash of the given data and returns it as a hex string.
func SHA256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// SHA256Bytes computes the SHA-256 hash of the given data and returns the raw bytes.
func SHA256Bytes(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// HMACSHA256 computes an HMAC-SHA256 of the data using the given key.
// Returns the MAC as raw bytes.
func HMACSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// HMACSHA256Hex computes an HMAC-SHA256 and returns the result as a hex string.
func HMACSHA256Hex(key, data []byte) string {
	return hex.EncodeToString(HMACSHA256(key, data))
}

// VerifyHMACSHA256 verifies an HMAC-SHA256 tag against expected data.
// Uses constant-time comparison to prevent timing attacks.
func VerifyHMACSHA256(key, data, expectedMAC []byte) bool {
	actualMAC := HMACSHA256(key, data)
	return hmac.Equal(actualMAC, expectedMAC)
}

// HexEncode encodes bytes to a hex string.
func HexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

// HexDecode decodes a hex string to bytes.
func HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// GenerateRandomInt returns a random integer in the range [min, max].
// Note: This is NOT cryptographically secure, but suitable for short-lived OTPs.
func GenerateRandomInt(min, max int) int {
	var b [4]byte
	rand.Read(b[:])
	val := int(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
	if val < 0 { val = -val }
	return min + (val % (max - min + 1))
}
