// Package utils provides shared utility functions for the Forge platform.
// All utilities are implemented using only the Go standard library.
package utils

import (
	"crypto/rand"
	"fmt"
)

// GenerateUUID creates a new UUID v4 (random) string.
// It follows RFC 4122 specification for version 4, variant 1 UUIDs.
// Format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
// where x is any hex digit and y is one of 8, 9, a, or b.
func GenerateUUID() (string, error) {
	var uuid [16]byte

	_, err := rand.Read(uuid[:])
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes for UUID: %w", err)
	}

	// Set version 4 (bits 12-15 of time_hi_and_version to 0100)
	uuid[6] = (uuid[6] & 0x0f) | 0x40

	// Set variant 1 (bits 6-7 of clock_seq_hi_and_reserved to 10)
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4],
		uuid[4:6],
		uuid[6:8],
		uuid[8:10],
		uuid[10:16],
	), nil
}

// MustGenerateUUID creates a new UUID v4 and panics if random generation fails.
// Use this only in contexts where a failure to generate a UUID is unrecoverable.
func MustGenerateUUID() string {
	id, err := GenerateUUID()
	if err != nil {
		panic("forge: failed to generate UUID: " + err.Error())
	}
	return id
}
