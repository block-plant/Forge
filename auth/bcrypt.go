// Package auth implements the Forge authentication service.
// All cryptographic primitives (bcrypt, JWT RS256) are implemented
// using only Go standard library packages.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"strconv"
)

// bcrypt constants
const (
	MinCost     = 4
	MaxCost     = 31
	DefaultCost = 12
)

// bcrypt errors
var (
	ErrHashTooShort   = errors.New("bcrypt: hash too short")
	ErrInvalidHash    = errors.New("bcrypt: invalid hash format")
	ErrInvalidCost    = errors.New("bcrypt: invalid cost")
	ErrMismatchedHash = errors.New("bcrypt: hashedPassword is not the hash of the given password")
)

// HashPassword hashes a password using bcrypt with the given cost.
// Returns a string in the format: $2b$<cost>$<22-char salt><31-char hash>
func HashPassword(password string, cost int) (string, error) {
	if cost < MinCost || cost > MaxCost {
		return "", ErrInvalidCost
	}

	// Generate 16 random bytes for the salt
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("bcrypt: failed to generate salt: %w", err)
	}

	hash, err := bcryptHash([]byte(password), cost, saltBytes)
	if err != nil {
		return "", err
	}

	return hash, nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil on success, ErrMismatchedHash on failure.
func CheckPassword(password, hash string) error {
	cost, salt, existingHash, err := parseBcryptHash(hash)
	if err != nil {
		return err
	}

	// Hash the password with the same cost and salt
	newHashStr, err := bcryptHash([]byte(password), cost, salt)
	if err != nil {
		return err
	}

	// Parse out just the hash portion of the new hash
	_, _, newHash, err := parseBcryptHash(newHashStr)
	if err != nil {
		return err
	}

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(existingHash, newHash) != 1 {
		return ErrMismatchedHash
	}

	return nil
}

// bcryptHash performs the core bcrypt operation.
func bcryptHash(password []byte, cost int, salt []byte) (string, error) {
	// Truncate password to 72 bytes (bcrypt limit)
	if len(password) > 72 {
		password = password[:72]
	}

	// Expensive key setup using Blowfish
	cipher := newBlowfish(expandKey(password, salt, cost))

	// Encrypt the magic string "OrpheanBeholderScryDoubt" (24 bytes = 3 × 8-byte blocks)
	magic := []byte("OrpheanBeholderScryDoubt")
	ciphertext := make([]byte, 24)
	copy(ciphertext, magic)

	// Encrypt each 8-byte block 64 times
	for i := 0; i < 24; i += 8 {
		for j := 0; j < 64; j++ {
			l := uint32(ciphertext[i])<<24 | uint32(ciphertext[i+1])<<16 | uint32(ciphertext[i+2])<<8 | uint32(ciphertext[i+3])
			r := uint32(ciphertext[i+4])<<24 | uint32(ciphertext[i+5])<<16 | uint32(ciphertext[i+6])<<8 | uint32(ciphertext[i+7])
			l, r = cipher.encrypt(l, r)
			ciphertext[i] = byte(l >> 24)
			ciphertext[i+1] = byte(l >> 16)
			ciphertext[i+2] = byte(l >> 8)
			ciphertext[i+3] = byte(l)
			ciphertext[i+4] = byte(r >> 24)
			ciphertext[i+5] = byte(r >> 16)
			ciphertext[i+6] = byte(r >> 8)
			ciphertext[i+7] = byte(r)
		}
	}

	// Format: $2b$<cost>$<22-char base64 salt><31-char base64 hash>
	costStr := fmt.Sprintf("%02d", cost)
	saltEncoded := bcryptBase64Encode(salt)    // 16 bytes → 22 chars
	hashEncoded := bcryptBase64Encode(ciphertext[:23]) // 23 bytes → 31 chars

	return "$2b$" + costStr + "$" + saltEncoded + hashEncoded, nil
}

// expandKey performs the expensive Blowfish key expansion with salt and cost.
func expandKey(password, salt []byte, cost int) *blowfishCipher {
	bf := blowfishSetup(password, salt)

	// Expensive key schedule: 2^cost iterations
	rounds := 1 << uint(cost)
	for i := 0; i < rounds; i++ {
		bf.expandKeyWithSalt(password)
		bf.expandKeyWithSalt(salt)
	}

	return bf
}

// parseBcryptHash extracts cost, salt, and hash from a bcrypt hash string.
func parseBcryptHash(hash string) (int, []byte, []byte, error) {
	if len(hash) < 60 {
		return 0, nil, nil, ErrHashTooShort
	}

	// Format: $2b$XX$<22 salt chars><31 hash chars>
	if hash[0] != '$' {
		return 0, nil, nil, ErrInvalidHash
	}

	// Find version (2a, 2b, or 2y)
	parts := splitDollar(hash[1:])
	if len(parts) < 3 {
		return 0, nil, nil, ErrInvalidHash
	}

	version := parts[0]
	if version != "2b" && version != "2a" && version != "2y" {
		return 0, nil, nil, ErrInvalidHash
	}

	cost, err := strconv.Atoi(parts[1])
	if err != nil || cost < MinCost || cost > MaxCost {
		return 0, nil, nil, ErrInvalidCost
	}

	// The rest is salt (22 chars) + hash (31 chars)
	saltAndHash := parts[2]
	if len(saltAndHash) < 53 {
		return 0, nil, nil, ErrHashTooShort
	}

	saltEncoded := saltAndHash[:22]
	hashEncoded := saltAndHash[22:]

	salt := bcryptBase64Decode(saltEncoded)
	hashBytes := bcryptBase64Decode(hashEncoded)

	return cost, salt, hashBytes, nil
}

// splitDollar splits a string by '$', handling the bcrypt format.
func splitDollar(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '$' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// ---- bcrypt Base64 (non-standard) ----
// bcrypt uses a custom Base64 alphabet: ./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789

const bcryptBase64Chars = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// bcryptBase64Encode encodes bytes using bcrypt's custom base64.
func bcryptBase64Encode(data []byte) string {
	result := make([]byte, 0, (len(data)*4+2)/3)

	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}

		result = append(result, bcryptBase64Chars[b0&0x3f])
		result = append(result, bcryptBase64Chars[((b0>>6)|(b1<<2))&0x3f])

		if i+1 < len(data) {
			result = append(result, bcryptBase64Chars[((b1>>4)|(b2<<4))&0x3f])
		}
		if i+2 < len(data) {
			result = append(result, bcryptBase64Chars[(b2>>2)&0x3f])
		}
	}

	return string(result)
}

// bcryptBase64Decode decodes bcrypt's custom base64.
func bcryptBase64Decode(s string) []byte {
	numBytes := (len(s) * 3) / 4
	result := make([]byte, numBytes)

	lookup := make([]byte, 256)
	for i := 0; i < len(bcryptBase64Chars); i++ {
		lookup[bcryptBase64Chars[i]] = byte(i)
	}

	j := 0
	for i := 0; i < len(s); i += 4 {
		var c0, c1, c2, c3 byte
		c0 = lookup[s[i]]
		if i+1 < len(s) {
			c1 = lookup[s[i+1]]
		}
		if i+2 < len(s) {
			c2 = lookup[s[i+2]]
		}
		if i+3 < len(s) {
			c3 = lookup[s[i+3]]
		}

		if j < numBytes {
			result[j] = c0 | (c1 << 6)
			j++
		}
		if j < numBytes {
			result[j] = (c1 >> 2) | (c2 << 4)
			j++
		}
		if j < numBytes {
			result[j] = (c2 >> 4) | (c3 << 2)
			j++
		}
	}

	return result
}

// ---- Blowfish Cipher (from scratch) ----
// Implements the Blowfish block cipher as described by Bruce Schneier.
// Used internally by bcrypt for key derivation.

type blowfishCipher struct {
	p [18]uint32
	s [4][256]uint32
}

// newBlowfish creates a Blowfish cipher from an already-expanded key state.
func newBlowfish(bf *blowfishCipher) *blowfishCipher {
	return bf
}

// encrypt performs Blowfish encryption on a 64-bit block (two 32-bit halves).
func (bf *blowfishCipher) encrypt(l, r uint32) (uint32, uint32) {
	for i := 0; i < 16; i += 2 {
		l ^= bf.p[i]
		r ^= bf.f(l)
		r ^= bf.p[i+1]
		l ^= bf.f(r)
	}
	l ^= bf.p[16]
	r ^= bf.p[17]
	return r, l
}

// f is the Blowfish F-function that mixes S-box lookups.
func (bf *blowfishCipher) f(x uint32) uint32 {
	a := bf.s[0][byte(x>>24)]
	b := bf.s[1][byte(x>>16)]
	c := bf.s[2][byte(x>>8)]
	d := bf.s[3][byte(x)]
	return ((a + b) ^ c) + d
}

// blowfishSetup initializes a Blowfish cipher with key and salt (for bcrypt).
func blowfishSetup(key, salt []byte) *blowfishCipher {
	bf := &blowfishCipher{}

	// Initialize P-array and S-boxes with digits of pi
	copy(bf.p[:], p0[:])
	copy(bf.s[0][:], s0[:])
	copy(bf.s[1][:], s1[:])
	copy(bf.s[2][:], s2[:])
	copy(bf.s[3][:], s3[:])

	// XOR P-array with key bytes (cycling through key)
	j := 0
	for i := 0; i < 18; i++ {
		var data uint32
		for k := 0; k < 4; k++ {
			data = (data << 8) | uint32(key[j])
			j++
			if j >= len(key) {
				j = 0
			}
		}
		bf.p[i] ^= data
	}

	// Encrypt with salt-mixed rounds to expand P-array
	var l, r uint32
	for i := 0; i < 18; i += 2 {
		l ^= saltWord(salt, i*4)
		r ^= saltWord(salt, i*4+4)
		l, r = bf.encrypt(l, r)
		bf.p[i] = l
		bf.p[i+1] = r
	}

	// Expand S-boxes
	for box := 0; box < 4; box++ {
		for i := 0; i < 256; i += 2 {
			l ^= saltWord(salt, (i+box*256)*4)
			r ^= saltWord(salt, (i+box*256)*4+4)
			l, r = bf.encrypt(l, r)
			bf.s[box][i] = l
			bf.s[box][i+1] = r
		}
	}

	return bf
}

// expandKeyWithSalt re-expands the key schedule using the given salt/key data.
func (bf *blowfishCipher) expandKeyWithSalt(key []byte) {
	j := 0
	for i := 0; i < 18; i++ {
		var data uint32
		for k := 0; k < 4; k++ {
			data = (data << 8) | uint32(key[j])
			j++
			if j >= len(key) {
				j = 0
			}
		}
		bf.p[i] ^= data
	}

	var l, r uint32
	for i := 0; i < 18; i += 2 {
		l, r = bf.encrypt(l, r)
		bf.p[i] = l
		bf.p[i+1] = r
	}

	for box := 0; box < 4; box++ {
		for i := 0; i < 256; i += 2 {
			l, r = bf.encrypt(l, r)
			bf.s[box][i] = l
			bf.s[box][i+1] = r
		}
	}
}

// saltWord extracts a 32-bit word from the salt at a given byte offset, wrapping.
func saltWord(salt []byte, offset int) uint32 {
	if len(salt) == 0 {
		return 0
	}
	var word uint32
	for i := 0; i < 4; i++ {
		word = (word << 8) | uint32(salt[(offset+i)%len(salt)])
	}
	return word
}
