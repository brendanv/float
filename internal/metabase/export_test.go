package metabase

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteSQL(t *testing.T) {
	in := []byte("CREATE TABLE \"postings\" (\nid serial,\ntxnidx bigint,\namount numeric);")
	out := string(rewriteSQL(in))
	if strings.Contains(out, "id serial") {
		t.Fatalf("expected 'id serial' to be rewritten, got: %s", out)
	}
	if !strings.Contains(out, "id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL") {
		t.Fatalf("expected autoincrement primary key, got: %s", out)
	}
}

// stubExporter returns canned SQL that mimics hledger's `print -O sql` output.
type stubExporter struct{ sql string }

func (s stubExporter) ExportSQL(context.Context) ([]byte, error) { return []byte(s.sql), nil }

func TestExport(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed; skipping export integration test")
	}

	const sql = `create table if not exists postings
(id serial, txnidx bigint, date1 date, description text, account text, amount numeric);
insert into postings(txnidx,date1,description,account,amount) values
(1,'2026-01-01','Opening','assets:cash',100),
(1,'2026-01-01','Opening','equity:opening',-100);
`
	dbPath := filepath.Join(t.TempDir(), "exports", "float.db")
	stats, err := Export(t.Context(), stubExporter{sql: sql}, dbPath)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if stats.PostingCount != 2 {
		t.Fatalf("expected 2 postings, got %d", stats.PostingCount)
	}
	if stats.SizeBytes == 0 {
		t.Fatalf("expected non-zero db size")
	}

	// The id column must be populated (the rewrite fix), not NULL.
	out, err := exec.Command("sqlite3", dbPath, "select count(*) from postings where id is null;").Output()
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "0" {
		t.Fatalf("expected 0 rows with NULL id, got %s", got)
	}
}
