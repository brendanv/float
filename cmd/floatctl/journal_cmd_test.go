package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/testgen"
)

// journalDataDir creates a small test data directory using testgen.
func journalDataDir(t *testing.T) string {
	t.Helper()
	return testgen.GenerateDataDir(t, testgen.Options{
		Seed:     42,
		NumTxns:  5,
		WithFIDs: true,
	})
}

func TestRunJournalLookup(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing args", args: []string{}, wantErr: "missing"},
		{name: "missing fid", args: []string{"/some/dir"}, wantErr: "missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runJournalLookup(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunJournalLookupFound(t *testing.T) {
	dir := journalDataDir(t)

	// Extract a real FID from the generated data
	mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
	if err != nil {
		t.Fatal(err)
	}
	// Find an include path to read a journal file and grab a FID
	var fid string
	for _, line := range strings.Split(string(mainData), "\n") {
		if strings.HasPrefix(line, "include ") {
			rel := strings.TrimPrefix(line, "include ")
			data, readErr := os.ReadFile(filepath.Join(dir, strings.TrimSpace(rel)))
			if readErr != nil {
				continue
			}
			for _, jline := range strings.Split(string(data), "\n") {
				if idx := strings.Index(jline, "fid:"); idx >= 0 {
					fid = jline[idx+4 : idx+12]
					break
				}
			}
		}
		if fid != "" {
			break
		}
	}
	if fid == "" {
		t.Fatal("could not extract a FID from generated journal")
	}

	flush := captureStdout(t)
	err = runJournalLookup([]string{dir, fid})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

func TestRunJournalLookupNotFound(t *testing.T) {
	dir := journalDataDir(t)
	err := runJournalLookup([]string{dir, "00000000"})
	if err == nil {
		t.Fatal("expected error for non-existent fid, got nil")
	}
	if !strings.Contains(err.Error(), "no transaction found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunJournalStats(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing arg", args: []string{}, wantErr: "missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runJournalStats(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestRunJournalStatsOutput(t *testing.T) {
	dir := journalDataDir(t)

	flush := captureStdout(t)
	err := runJournalStats([]string{dir})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)

	// Verify key fields are present in the output
	for _, field := range []string{"journal_files", "transactions", "accounts", "total_size_bytes"} {
		if !strings.Contains(out, `"`+field+`"`) {
			t.Errorf("stats output missing field %q:\n%s", field, out)
		}
	}
}

func TestRunJournalAudit(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing arg", args: []string{}, wantErr: "missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runJournalAudit(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestRunJournalAuditClean(t *testing.T) {
	dir := journalDataDir(t)

	flush := captureStdout(t)
	err := runJournalAudit([]string{dir})
	out := flush()
	if err != nil {
		t.Fatalf("clean dir: unexpected error: %v", err)
	}
	assertValidJSON(t, out)
	if !strings.Contains(out, `"ok": true`) {
		t.Errorf("expected ok:true for clean dir, got:\n%s", out)
	}
}

func TestRunJournalAuditMissingInclude(t *testing.T) {
	dir := t.TempDir()
	// Create a main.journal that references a non-existent file
	mainContent := "include 2026/01.journal\n"
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}

	flush := captureStdout(t)
	err := runJournalAudit([]string{dir})
	out := flush()
	// Should return an error (ok:false means non-zero exit)
	if err == nil {
		t.Fatal("expected error for missing include, got nil")
	}
	assertValidJSON(t, out)
	if !strings.Contains(out, `"ok": false`) {
		t.Errorf("expected ok:false, got:\n%s", out)
	}
	if !strings.Contains(out, "2026/01.journal") {
		t.Errorf("expected missing file in output, got:\n%s", out)
	}
}

func TestRunJournalAuditDuplicateFID(t *testing.T) {
	dir := t.TempDir()
	// Two transactions in the same file with the same FID
	journalContent := "2026/01.journal"
	txns := "2026-01-01 TEST A  ; fid:aa001100\n    expenses:misc  $1.00\n    assets:checking\n\n" +
		"2026-01-02 TEST B  ; fid:aa001100\n    expenses:misc  $2.00\n    assets:checking\n"
	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026/01.journal"), []byte(txns), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("include "+journalContent+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	flush := captureStdout(t)
	err := runJournalAudit([]string{dir})
	out := flush()
	if err == nil {
		t.Fatal("expected error for duplicate FID, got nil")
	}
	assertValidJSON(t, out)
	if !strings.Contains(out, "aa001100") {
		t.Errorf("expected duplicate fid in output, got:\n%s", out)
	}
	if !strings.Contains(out, `"ok": false`) {
		t.Errorf("expected ok:false, got:\n%s", out)
	}
}

func TestRunHledgerRaw(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErr    string
		wantAnyErr bool
	}{
		{name: "missing journal", args: []string{}, wantErr: "missing"},
		{name: "missing subcmd", args: []string{testdataPath("simple.journal")}, wantErr: "missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runHledgerRaw(tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if tc.wantAnyErr && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestRunHledgerRawOutput(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerRaw([]string{testdataPath("simple.journal"), "bal"})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// `hledger bal` without -O json returns plain text; just verify it's non-empty
	if strings.TrimSpace(out) == "" {
		t.Error("expected non-empty output from hledger bal")
	}
}
