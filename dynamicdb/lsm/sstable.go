package lsm

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const (
	// SSTMagic is the magic number used for identifying valid SSTable files.
	SSTMagic        uint32 = 0x44594E41 // "DYNA"
	// DefaultBlockSize represents the byte limit for chunking payload data blocks.
	DefaultBlockSize       = 4096
)

// SSTableBuilder sequentially writes sorted key-value pairs into immutable disk blocks.
type SSTableBuilder struct {
	file         *os.File
	bw           *bufio.Writer
	offset       int64
	blockSize    int
	
	// Current block buffer
	blockBuf     bytes.Buffer
	blockEntries int
	firstKey     []byte // First key of the current block
	
	// Index mappings (Block start key -> Block offset/size)
	indexBlock   bytes.Buffer
	
	// Keys collected for the Bloom Filter
	keys         [][]byte
}

func NewSSTableBuilder(path string) (*SSTableBuilder, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	return &SSTableBuilder{
		file:      f,
		bw:        bufio.NewWriter(f),
		blockSize: DefaultBlockSize,
	}, nil
}

// Add appends a key-value pair to the SSTable. Keys MUST be sorted.
func (b *SSTableBuilder) Add(key, value []byte, eType EntryType) error {
	// Add key for bloom filter
	b.keys = append(b.keys, key)
	
	if b.blockEntries == 0 {
		b.firstKey = make([]byte, len(key))
		copy(b.firstKey, key)
	}

	// Write Entry to Block Buffer: [kLen 4][key][vLen 4][value][type 1]
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(key)))
	b.blockBuf.Write(lenBuf[:])
	b.blockBuf.Write(key)

	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(value)))
	b.blockBuf.Write(lenBuf[:])
	b.blockBuf.Write(value)

	b.blockBuf.WriteByte(byte(eType))
	b.blockEntries++

	if b.blockBuf.Len() >= b.blockSize {
		return b.flushBlock()
	}
	return nil
}

// flushBlock writes the current data block to disk and updates the index.
func (b *SSTableBuilder) flushBlock() error {
	if b.blockEntries == 0 {
		return nil
	}

	// Add Block header (4 bytes: count)
	var countBuf [4]byte
	binary.LittleEndian.PutUint32(countBuf[:], uint32(b.blockEntries))
	
	blockData := append(countBuf[:], b.blockBuf.Bytes()...)
	
	// Write block to disk
	n, err := b.bw.Write(blockData)
	if err != nil {
		return err
	}

	// Write Index Entry: [kLen 4][firstKey][offset 8][size 4]
	binary.LittleEndian.PutUint32(countBuf[:], uint32(len(b.firstKey)))
	b.indexBlock.Write(countBuf[:])
	b.indexBlock.Write(b.firstKey)

	var offsizeBuf [12]byte
	binary.LittleEndian.PutUint64(offsizeBuf[0:8], uint64(b.offset))
	binary.LittleEndian.PutUint32(offsizeBuf[8:12], uint32(n))
	b.indexBlock.Write(offsizeBuf[:])

	// Reset block state
	b.offset += int64(n)
	b.blockBuf.Reset()
	b.blockEntries = 0
	return nil
}

// Finish flushes remaining data, writes Index/Bloom, and finalizes the file.
func (b *SSTableBuilder) Finish() error {
	if err := b.flushBlock(); err != nil {
		return err
	}

	indexOffset := b.offset
	idxBytes := b.indexBlock.Bytes()
	nIdx, err := b.bw.Write(idxBytes)
	if err != nil {
		return err
	}
	b.offset += int64(nIdx)

	// Build and Write Bloom Filter
	filter := NewBloomFilter(len(b.keys), 10)
	for _, key := range b.keys {
		filter.Add(key)
	}

	bloomOffset := b.offset
	nBloom, err := b.bw.Write(filter)
	if err != nil {
		return err
	}
	b.offset += int64(nBloom)

	// Write Footer: [IndexOffset 8][BloomOffset 8][Magic 4]
	var footer [20]byte
	binary.LittleEndian.PutUint64(footer[0:8], uint64(indexOffset))
	binary.LittleEndian.PutUint64(footer[8:16], uint64(bloomOffset))
	binary.LittleEndian.PutUint32(footer[16:20], SSTMagic)

	if _, err := b.bw.Write(footer[:]); err != nil {
		return err
	}

	if err := b.bw.Flush(); err != nil {
		return err
	}

	return b.file.Close()
}

// indexEntry maps a starting key to its block coordinates.
type indexEntry struct {
	key    []byte
	offset int64
	size   int32
}

// SSTableReader allows querying a finished SSTable.
type SSTableReader struct {
	file   *os.File
	filter BloomFilter
	index  []indexEntry
}

// OpenSSTable loads the filter and index from disk.
func OpenSSTable(path string) (*SSTableReader, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := stat.Size()
	if size < 20 {
		return nil, io.ErrUnexpectedEOF
	}

	// Read Footer
	var footer [20]byte
	if _, err := f.ReadAt(footer[:], size-20); err != nil {
		return nil, err
	}

	magic := binary.LittleEndian.Uint32(footer[16:20])
	if magic != SSTMagic {
		return nil, errors.New("sstable: invalid magic number")
	}

	indexOffset := int64(binary.LittleEndian.Uint64(footer[0:8]))
	bloomOffset := int64(binary.LittleEndian.Uint64(footer[8:16]))

	// Load Bloom Filter
	bloomSize := size - 20 - bloomOffset
	filter := make([]byte, bloomSize)
	if _, err := f.ReadAt(filter, bloomOffset); err != nil {
		return nil, err
	}

	// Load Index Block
	indexSize := bloomOffset - indexOffset
	idxBytes := make([]byte, indexSize)
	if _, err := f.ReadAt(idxBytes, indexOffset); err != nil {
		return nil, err
	}

	var index []indexEntry
	r := bytes.NewReader(idxBytes)
	for r.Len() > 0 {
		var kLen uint32
		if err := binary.Read(r, binary.LittleEndian, &kLen); err != nil {
			break
		}
		
		key := make([]byte, kLen)
		io.ReadFull(r, key)

		var offsize [12]byte
		io.ReadFull(r, offsize[:])
		
		offset := int64(binary.LittleEndian.Uint64(offsize[0:8]))
		sz := int32(binary.LittleEndian.Uint32(offsize[8:12]))

		index = append(index, indexEntry{key: key, offset: offset, size: sz})
	}

	return &SSTableReader{
		file:   f,
		filter: filter,
		index:  index,
	}, nil
}

// Get queries the SSTable. False indicates key is definitively absent.
func (s *SSTableReader) Get(key []byte) ([]byte, bool, EntryType, error) {
	if !s.filter.MayContain(key) {
		return nil, false, 0, nil
	}

	// Binary search the sparse index to find the containing block
	blockIdx := -1
	low, high := 0, len(s.index)-1
	for low <= high {
		mid := low + (high-low)/2
		cmp := bytes.Compare(s.index[mid].key, key)
		if cmp == 0 {
			blockIdx = mid
			break
		} else if cmp < 0 {
			blockIdx = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	if blockIdx == -1 {
		return nil, false, 0, nil
	}

	// Read block
	entry := s.index[blockIdx]
	blockData := make([]byte, entry.size)
	if _, err := s.file.ReadAt(blockData, entry.offset); err != nil {
		return nil, false, 0, err
	}

	// Scan through the linear block list
	count := binary.LittleEndian.Uint32(blockData[0:4])
	pos := 4

	for i := uint32(0); i < count; i++ {
		kLen := binary.LittleEndian.Uint32(blockData[pos : pos+4])
		pos += 4
		k := blockData[pos : pos+int(kLen)]
		pos += int(kLen)

		vLen := binary.LittleEndian.Uint32(blockData[pos : pos+4])
		pos += 4
		v := blockData[pos : pos+int(vLen)]
		pos += int(vLen)

		eType := EntryType(blockData[pos])
		pos++

		if bytes.Equal(k, key) {
			return v, true, eType, nil
		}
	}

	return nil, false, 0, nil
}

// Close closes the file descriptor.
func (s *SSTableReader) Close() error {
	return s.file.Close()
}
