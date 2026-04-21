package lsm

import (
	"hash/fnv"
)

// BloomFilter is a space-efficient probabilistic data structure that is used 
// to test whether a key is *not* in an SSTable, saving expensive disk reads.
type BloomFilter []byte

// NewBloomFilter creates a filter of m bits. 
// bitsPerKey is typically around 10 for a 1% false positive rate.
func NewBloomFilter(numKeys, bitsPerKey int) BloomFilter {
	if numKeys < 0 {
		numKeys = 0
	}
	bits := numKeys * bitsPerKey
	if bits < 64 {
		bits = 64
	}
	bytes := (bits + 7) / 8
	// Store the 'k' (numHashFunctions) in the last byte.
	// For bitsPerKey=10, optimal k is about 6 or 7
	k := uint8(float64(bitsPerKey) * 0.69)
	if k < 1 {
		k = 1
	} else if k > 30 {
		k = 30
	}

	filter := make([]byte, bytes+1)
	filter[bytes] = k
	return filter
}

// Add adds a key to the bloom filter.
func (f BloomFilter) Add(key []byte) {
	if len(f) < 2 {
		return
	}
	k := int(f[len(f)-1])
	bits := (len(f) - 1) * 8

	h1, h2 := hash(key)
	for i := 0; i < k; i++ {
		bitPos := (h1 + uint32(i)*h2) % uint32(bits)
		f[bitPos/8] |= (1 << (bitPos % 8))
	}
}

// MayContain checks if the key might be in the set.
// It returns false if the key is definitely not in the set.
func (f BloomFilter) MayContain(key []byte) bool {
	if len(f) < 2 {
		return false
	}
	k := int(f[len(f)-1])
	bits := (len(f) - 1) * 8

	h1, h2 := hash(key)
	for i := 0; i < k; i++ {
		bitPos := (h1 + uint32(i)*h2) % uint32(bits)
		if f[bitPos/8]&(1<<(bitPos%8)) == 0 {
			return false
		}
	}
	return true
}

// hash generates two 32-bit hashes using FNV-1a 64-bit to simulate k hash functions.
func hash(key []byte) (uint32, uint32) {
	h := fnv.New64a()
	h.Write(key)
	sum := h.Sum64()
	return uint32(sum >> 32), uint32(sum)
}
