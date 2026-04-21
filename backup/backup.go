// Package backup implements full-platform backup and restore for the Forge platform.
// A backup is a single self-describing JSON archive containing a snapshot of every
// active service: database documents, auth users, storage metadata, analytics counters,
// and server version information.
//
// Backup files use the extension .forge-backup and are gzip-compressed JSON.
package backup

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Version stamps every backup so the restore logic can handle format changes.
const BackupVersion = 1

// Manifest is the top-level structure of a Forge backup file.
type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	ForgeVer  string    `json:"forge_version"`
	DataDir   string    `json:"data_dir"`

	Database  DatabaseBackup  `json:"database"`
	Auth      AuthBackup      `json:"auth"`
	Storage   StorageBackup   `json:"storage"`
	Analytics AnalyticsBackup `json:"analytics"`
}

// DatabaseBackup holds all documents grouped by collection.
type DatabaseBackup struct {
	Collections map[string][]map[string]interface{} `json:"collections"` // col → []doc
	IndexDefs   []IndexDefinition                   `json:"index_defs,omitempty"`
}

// IndexDefinition describes a composite index to recreate on restore.
type IndexDefinition struct {
	Collection string   `json:"collection"`
	Fields     []string `json:"fields"`
}

// AuthBackup holds all user records.
type AuthBackup struct {
	Users []map[string]interface{} `json:"users"`
}

// StorageBackup holds file metadata (actual blobs stay on disk, referenced by path).
type StorageBackup struct {
	Files []map[string]interface{} `json:"files"`
}

// AnalyticsBackup holds aggregated event counters.
type AnalyticsBackup struct {
	Counters map[string]int64 `json:"counters"`
}

// ---- Writer ----

// Writer collects service data and writes a backup file.
type Writer struct {
	manifest Manifest
}

// NewWriter creates a new backup writer initialised with server metadata.
func NewWriter(forgeVersion, dataDir string) *Writer {
	return &Writer{
		manifest: Manifest{
			Version:   BackupVersion,
			CreatedAt: time.Now().UTC(),
			ForgeVer:  forgeVersion,
			DataDir:   dataDir,
			Database: DatabaseBackup{
				Collections: make(map[string][]map[string]interface{}),
			},
			Auth:    AuthBackup{},
			Storage: StorageBackup{},
			Analytics: AnalyticsBackup{
				Counters: make(map[string]int64),
			},
		},
	}
}

// AddCollection adds all documents from a collection to the backup.
func (w *Writer) AddCollection(name string, docs []map[string]interface{}) {
	w.manifest.Database.Collections[name] = docs
}

// AddIndexDef records a composite index definition.
func (w *Writer) AddIndexDef(collection string, fields []string) {
	w.manifest.Database.IndexDefs = append(w.manifest.Database.IndexDefs, IndexDefinition{
		Collection: collection,
		Fields:     fields,
	})
}

// AddUsers adds all auth user records to the backup.
func (w *Writer) AddUsers(users []map[string]interface{}) {
	w.manifest.Auth.Users = users
}

// AddStorageFiles adds storage file metadata to the backup.
func (w *Writer) AddStorageFiles(files []map[string]interface{}) {
	w.manifest.Storage.Files = files
}

// AddAnalyticsCounter adds a named counter value to the backup.
func (w *Writer) AddAnalyticsCounter(name string, value int64) {
	w.manifest.Analytics.Counters[name] = value
}

// WriteToStream writes the gzip-compressed JSON backup to the given writer.
func (w *Writer) WriteToStream(out io.Writer) error {
	gz := gzip.NewWriter(out)
	defer gz.Close()

	enc := json.NewEncoder(gz)
	enc.SetIndent("", "  ")
	return enc.Encode(w.manifest)
}

// WriteFile writes the backup to a file, creating parent directories as needed.
// Returns the final path written.
func (w *Writer) WriteFile(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("backup: failed to create output dir: %w", err)
	}

	ts := w.manifest.CreatedAt.Format("2006-01-02T15-04-05")
	name := fmt.Sprintf("forge-backup-%s.forge-backup", ts)
	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("backup: failed to create file: %w", err)
	}
	defer f.Close()

	if err := w.WriteToStream(f); err != nil {
		return "", fmt.Errorf("backup: write failed: %w", err)
	}

	return path, nil
}

// ---- Reader ----

// ReadFile reads and decompresses a backup file, returning the Manifest.
func ReadFile(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("backup: failed to open file: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("backup: not a valid gzip file: %w", err)
	}
	defer gz.Close()

	var m Manifest
	if err := json.NewDecoder(gz).Decode(&m); err != nil {
		return nil, fmt.Errorf("backup: failed to decode manifest: %w", err)
	}

	if m.Version != BackupVersion {
		return nil, fmt.Errorf("backup: unsupported version %d (expected %d)", m.Version, BackupVersion)
	}

	return &m, nil
}

// Summary returns a human-readable summary of what's in this backup.
func (m *Manifest) Summary() string {
	totalDocs := 0
	for _, docs := range m.Database.Collections {
		totalDocs += len(docs)
	}
	return fmt.Sprintf(
		"Forge Backup v%d | Created: %s | ForgeVer: %s | Collections: %d | Docs: %d | Users: %d | StorageFiles: %d",
		m.Version,
		m.CreatedAt.Format(time.RFC3339),
		m.ForgeVer,
		len(m.Database.Collections),
		totalDocs,
		len(m.Auth.Users),
		len(m.Storage.Files),
	)
}
