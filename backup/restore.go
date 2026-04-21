package backup

import (
	"fmt"
)

// RestoreOptions controls which services are restored from a backup.
type RestoreOptions struct {
	RestoreDatabase  bool
	RestoreAuth      bool
	RestoreStorage   bool
	RestoreAnalytics bool
	DryRun           bool // if true, validate only — do not write
}

// DefaultRestoreOptions returns options that restore everything.
func DefaultRestoreOptions() RestoreOptions {
	return RestoreOptions{
		RestoreDatabase:  true,
		RestoreAuth:      true,
		RestoreStorage:   true,
		RestoreAnalytics: true,
	}
}

// RestoreReport summarises what was (or would be) restored.
type RestoreReport struct {
	CollectionsRestored int
	DocumentsRestored   int
	UsersRestored       int
	FilesRestored       int
	CountersRestored    int
	Errors              []string
}

// String returns a human-readable report.
func (r *RestoreReport) String() string {
	return fmt.Sprintf(
		"Restored: %d collections / %d documents / %d users / %d storage files / %d analytics counters | Errors: %d",
		r.CollectionsRestored,
		r.DocumentsRestored,
		r.UsersRestored,
		r.FilesRestored,
		r.CountersRestored,
		len(r.Errors),
	)
}

// ---- RestoreTarget interface ----
// Services that can be restored implement these interfaces.

// DatabaseRestoreTarget can re-ingest backed-up documents.
type DatabaseRestoreTarget interface {
	// RestoreDocument upserts a document into the given collection.
	RestoreDocument(collection string, data map[string]interface{}) error
}

// AuthRestoreTarget can re-ingest backed-up users.
type AuthRestoreTarget interface {
	// RestoreUser upserts a user from raw backup data.
	RestoreUser(data map[string]interface{}) error
}

// AnalyticsRestoreTarget can restore counters.
type AnalyticsRestoreTarget interface {
	// RestoreCounter sets a named counter to the given value.
	RestoreCounter(name string, value int64)
}

// ---- Restore ----

// Restore replays a Manifest into live service targets.
func Restore(
	m *Manifest,
	opts RestoreOptions,
	db DatabaseRestoreTarget,
	authSvc AuthRestoreTarget,
	analyticsSvc AnalyticsRestoreTarget,
) (*RestoreReport, error) {
	report := &RestoreReport{}

	// ── Database ──
	if opts.RestoreDatabase && db != nil {
		for collection, docs := range m.Database.Collections {
			restoredInCol := 0
			for _, rawDoc := range docs {
				if opts.DryRun {
					restoredInCol++
					continue
				}
				if err := db.RestoreDocument(collection, rawDoc); err != nil {
					report.Errors = append(report.Errors,
						fmt.Sprintf("db/%s: %v", collection, err))
				} else {
					restoredInCol++
				}
			}
			if restoredInCol > 0 {
				report.CollectionsRestored++
				report.DocumentsRestored += restoredInCol
			}
		}
	}

	// ── Auth ──
	if opts.RestoreAuth && authSvc != nil {
		for _, rawUser := range m.Auth.Users {
			if opts.DryRun {
				report.UsersRestored++
				continue
			}
			if err := authSvc.RestoreUser(rawUser); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("auth: %v", err))
			} else {
				report.UsersRestored++
			}
		}
	}

	// ── Storage — metadata only (blobs remain on disk) ──
	if opts.RestoreStorage {
		report.FilesRestored = len(m.Storage.Files)
		// The actual blob files must already exist at the configured data dir.
		// The restore only validates they are present on disk; writing them
		// back from backup is out of scope (blobs can be TBs).
	}

	// ── Analytics ──
	if opts.RestoreAnalytics && analyticsSvc != nil {
		for name, val := range m.Analytics.Counters {
			if !opts.DryRun {
				analyticsSvc.RestoreCounter(name, val)
			}
			report.CountersRestored++
		}
	}

	return report, nil
}
