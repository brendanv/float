// Package metabase wires float to a self-hosted Metabase instance for the
// Custom Dashboards feature. It builds a SQLite snapshot of the hledger ledger
// (via hledger's own SQL output) and talks to Metabase's REST API to keep the
// matching database connection provisioned and synced. No accounting logic
// lives here — hledger produces the SQL, sqlite3 materializes it, and Metabase
// owns all querying/visualization.
package metabase

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sqliteBin is the sqlite3 CLI used to materialize hledger's SQL into a
// database file. Overridable in tests.
var sqliteBin = "sqlite3"

// exportMu serializes concurrent exports. The export writes a derived artifact
// (not journal files), so it does not go through txlock; a plain mutex is
// enough to prevent two regenerations from racing on the same output file.
var exportMu sync.Mutex

// SQLExporter produces hledger's `print -O sql` output. *hledger.Client
// satisfies this; tests can substitute a stub.
type SQLExporter interface {
	ExportSQL(ctx context.Context) ([]byte, error)
}

// ExportStats describes a completed SQLite export.
type ExportStats struct {
	DBPath       string
	GeneratedAt  time.Time
	PostingCount int
	SizeBytes    int64
}

// rewriteSQL adapts hledger's Postgres-flavored SQL for SQLite. hledger emits
// `id serial` for the primary key, which SQLite does not understand (leaving
// the id column NULL); rewrite it to an autoincrementing integer primary key.
func rewriteSQL(sql []byte) []byte {
	return bytes.ReplaceAll(sql,
		[]byte("id serial"),
		[]byte("id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL"))
}

// Export regenerates the SQLite snapshot at dbPath from the current ledger.
// It builds into a temp file in the same directory and atomically renames over
// dbPath, so readers (Metabase) never observe a half-written database. hledger's
// SQL assumes an empty database, so each call produces a fresh file.
func Export(ctx context.Context, hl SQLExporter, dbPath string) (ExportStats, error) {
	exportMu.Lock()
	defer exportMu.Unlock()

	sql, err := hl.ExportSQL(ctx)
	if err != nil {
		return ExportStats{}, fmt.Errorf("metabase: export sql: %w", err)
	}
	sql = rewriteSQL(sql)

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ExportStats{}, fmt.Errorf("metabase: create export dir: %w", err)
	}

	// Reserve a unique temp path in the same dir, then remove the empty file so
	// sqlite3 creates the database fresh (the SQL expects an empty database).
	tmp, err := os.CreateTemp(dir, ".float-export-*.db.tmp")
	if err != nil {
		return ExportStats{}, fmt.Errorf("metabase: create temp db: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(tmpPath)
	defer func() { _ = os.Remove(tmpPath) }()

	cmd := exec.CommandContext(ctx, sqliteBin, tmpPath)
	cmd.Stdin = bytes.NewReader(sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ExportStats{}, fmt.Errorf("metabase: sqlite3 import failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	count, _ := postingCount(ctx, tmpPath)

	if err := os.Rename(tmpPath, dbPath); err != nil {
		return ExportStats{}, fmt.Errorf("metabase: finalize db: %w", err)
	}

	stats := ExportStats{
		DBPath:       dbPath,
		GeneratedAt:  time.Now(),
		PostingCount: count,
	}
	if info, err := os.Stat(dbPath); err == nil {
		stats.SizeBytes = info.Size()
	}
	return stats, nil
}

// postingCount returns the number of rows in the postings table, best-effort.
func postingCount(ctx context.Context, dbPath string) (int, error) {
	out, err := exec.CommandContext(ctx, sqliteBin, dbPath, "select count(*) from postings;").Output()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
