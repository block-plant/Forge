package lsm

import (
	"bufio"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

var (
	ErrInvalidCRC = errors.New("wal: invalid crc")
	ErrShortRead  = errors.New("wal: short read")
)

// WAL represents an append-only Write-Ahead Log.
// Format per record: 
// [CRC32 (4 bytes)] [TotalLen (4 bytes)] [Type (1 byte)] [KeyLen (4 bytes)] [Key] [ValLen (4 bytes)] [Value]
type WAL struct {
	mu   sync.Mutex
	file *os.File
	bw   *bufio.Writer
}

// NewWAL creates or opens a Write-Ahead Log for the LSM MemTable crash recovery.
func NewWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	return &WAL{
		file: f,
		bw:   bufio.NewWriter(f),
	}, nil
}

// Write appends an entry to the log and flushes to disk.
func (w *WAL) Write(key, value []byte, eType EntryType) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Lengths
	kLen := uint32(len(key))
	vLen := uint32(len(value))
	// type (1) + klen (4) + key + vlen (4) + value
	payloadLen := 1 + 4 + kLen + 4 + vLen

	// Payload buffer
	payload := make([]byte, payloadLen)
	payload[0] = byte(eType)
	binary.LittleEndian.PutUint32(payload[1:5], kLen)
	copy(payload[5:5+kLen], key)
	
	valOffset := 5 + kLen
	binary.LittleEndian.PutUint32(payload[valOffset:valOffset+4], vLen)
	copy(payload[valOffset+4:], value)

	crc := crc32.ChecksumIEEE(payload)

	// Write Header: CRC (4) + TotalLen (4)
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[0:4], crc)
	binary.LittleEndian.PutUint32(header[4:8], payloadLen)

	if _, err := w.bw.Write(header); err != nil {
		return err
	}
	if _, err := w.bw.Write(payload); err != nil {
		return err
	}
	
	if err := w.bw.Flush(); err != nil {
		return err
	}
	// Note: We avoid os.File.Sync() physically on every write here for performance, 
	// relying on OS buffers. A strict durability option would fsync.
	return nil
}

// Close gracefully closes the WAL.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bw.Flush()
	return w.file.Close()
}

// Replay reads the entire WAL and invokes fn for each valid entry.
func (w *WAL) Replay(fn func(key, value []byte, eType EntryType) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Seek(0, 0); err != nil {
		return err
	}

	reader := bufio.NewReader(w.file)
	header := make([]byte, 8)

	for {
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		expectedCRC := binary.LittleEndian.Uint32(header[0:4])
		payloadLen := binary.LittleEndian.Uint32(header[4:8])

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return err // Could be ErrUnexpectedEOF indicating a crash midway
		}

		actualCRC := crc32.ChecksumIEEE(payload)
		if actualCRC != expectedCRC {
			return ErrInvalidCRC
		}

		eType := EntryType(payload[0])
		kLen := binary.LittleEndian.Uint32(payload[1:5])
		key := payload[5 : 5+kLen]
		
		valOffset := 5 + kLen
		vLen := binary.LittleEndian.Uint32(payload[valOffset : valOffset+4])
		value := payload[valOffset+4 : valOffset+4+vLen]

		if err := fn(key, value, eType); err != nil {
			return err
		}
	}

	// Seek back to end to resume appending
	_, err := w.file.Seek(0, 2)
	return err
}

// Truncate clears the WAL after a MemTable flush to SSTable.
func (w *WAL) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bw.Flush()
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	_, err := w.file.Seek(0, 0)
	return err
}
