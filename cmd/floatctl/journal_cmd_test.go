package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/config"
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

func TestRunJournalDelete(t *testing.T) {
	t.Run("missing args", func(t *testing.T) {
		tests := []struct {
			name    string
			args    []string
			wantErr string
		}{
			{name: "no args", args: []string{}, wantErr: "missing"},
			{name: "only data-dir", args: []string{"/some/dir"}, wantErr: "missing"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := runJournalDelete(tc.args)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
			})
		}
	})

	t.Run("delete existing transaction", func(t *testing.T) {
		dir := journalDataDir(t)

		// Extract a real FID from the generated data.
		fid := extractFirstFID(t, dir)

		flush := captureStdout(t)
		err := runJournalDelete([]string{dir, fid})
		out := flush()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, fid) {
			t.Errorf("expected fid %q in output, got: %s", fid, out)
		}

		// Subsequent lookup should fail.
		err = runJournalLookup([]string{dir, fid})
		if err == nil || !strings.Contains(err.Error(), "no transaction found") {
			t.Errorf("expected 'no transaction found' after delete, got: %v", err)
		}
	})
}

func TestRunJournalAdd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no args", args: []string{}, wantErr: "missing"},
		{name: "missing description", args: []string{t.TempDir()}, wantErr: "--description"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runJournalAdd(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got error %v, want containing %q", err, tc.wantErr)
			}
		})
	}

	t.Run("too few postings", func(t *testing.T) {
		dir := journalDataDir(t)
		err := runJournalAdd([]string{dir,
			"--description", "Test",
			"--posting", "expenses:food  $5.00",
		})
		if err == nil || !strings.Contains(err.Error(), "2 --posting") {
			t.Errorf("got error %v, want containing '2 --posting'", err)
		}
	})

	t.Run("valid add", func(t *testing.T) {
		dir := journalDataDir(t)

		flush := captureStdout(t)
		err := runJournalAdd([]string{dir,
			"--date", "2026-02-15",
			"--description", "TEST TRANSACTION",
			"--posting", "expenses:food  $12.34",
			"--posting", "assets:checking",
		})
		out := flush()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "added transaction with fid:") {
			t.Errorf("expected fid in output, got: %s", out)
		}

		// Extract fid from output and look it up.
		parts := strings.Fields(out)
		fid := parts[len(parts)-1]
		err = runJournalLookup([]string{dir, fid})
		if err != nil {
			t.Errorf("lookup after add failed: %v", err)
		}
	})
}

func TestRunJournalImport(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no args", args: []string{}, wantErr: "missing"},
		{name: "only data-dir", args: []string{"/some/dir"}, wantErr: "missing"},
		{name: "missing profile", args: []string{"/some/dir", "/some/csv"}, wantErr: "--profile"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := runJournalImport(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got error %v, want containing %q", err, tc.wantErr)
			}
		})
	}

	t.Run("unknown profile", func(t *testing.T) {
		dir := journalDataDir(t)
		// Write a minimal config.toml with no bank profiles.
		cfg := &config.Config{}
		if err := config.Save(filepath.Join(dir, "config.toml"), cfg); err != nil {
			t.Fatal(err)
		}
		err := runJournalImport([]string{dir, testdataPath("import.csv"),
			"--profile", "nonexistent",
			"--yes",
		})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("got error %v, want containing 'not found'", err)
		}
	})

	t.Run("valid import", func(t *testing.T) {
		dir := journalDataDir(t)

		// Write config.toml with a bank profile pointing to testdata rules.
		rulesRel := "rules/test.rules"
		rulesAbs := filepath.Join(dir, rulesRel)
		if err := os.MkdirAll(filepath.Dir(rulesAbs), 0755); err != nil {
			t.Fatal(err)
		}
		// Copy testdata rules file.
		rulesData, err := os.ReadFile(testdataPath("import.rules"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rulesAbs, rulesData, 0644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "testbank", RulesFile: rulesRel},
			},
		}
		if err := config.Save(filepath.Join(dir, "config.toml"), cfg); err != nil {
			t.Fatal(err)
		}

		flush := captureStdout(t)
		err = runJournalImport([]string{dir, testdataPath("import.csv"),
			"--profile", "testbank",
			"--yes",
		})
		out := flush()
		if err != nil {
			t.Fatalf("unexpected error: %v\noutput: %s", err, out)
		}
		if !strings.Contains(out, "imported") {
			t.Errorf("expected 'imported' in output, got: %s", out)
		}
	})

	t.Run("duplicate detection", func(t *testing.T) {
		dir := journalDataDir(t)

		rulesRel := "rules/test.rules"
		rulesAbs := filepath.Join(dir, rulesRel)
		if err := os.MkdirAll(filepath.Dir(rulesAbs), 0755); err != nil {
			t.Fatal(err)
		}
		rulesData, err := os.ReadFile(testdataPath("import.rules"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rulesAbs, rulesData, 0644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "testbank", RulesFile: rulesRel},
			},
		}
		if err := config.Save(filepath.Join(dir, "config.toml"), cfg); err != nil {
			t.Fatal(err)
		}

		// First import.
		flush := captureStdout(t)
		if err := runJournalImport([]string{dir, testdataPath("import.csv"),
			"--profile", "testbank", "--yes",
		}); err != nil {
			_ = flush()
			t.Fatalf("first import: %v", err)
		}
		_ = flush()

		// Second import of same CSV — all should be duplicates.
		flush = captureStdout(t)
		if err := runJournalImport([]string{dir, testdataPath("import.csv"),
			"--profile", "testbank", "--yes",
		}); err != nil {
			_ = flush()
			t.Fatalf("second import: %v", err)
		}
		out := flush()
		if !strings.Contains(out, "nothing to import") {
			t.Errorf("expected 'nothing to import' on second run, got: %s", out)
		}
	})
}

// extractFirstFID reads main.journal and returns the first fid tag found.
func extractFirstFID(t *testing.T, dir string) string {
	t.Helper()
	mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(mainData), "\n") {
		if strings.HasPrefix(line, "include ") {
			rel := strings.TrimPrefix(line, "include ")
			data, readErr := os.ReadFile(filepath.Join(dir, strings.TrimSpace(rel)))
			if readErr != nil {
				continue
			}
			for _, jline := range strings.Split(string(data), "\n") {
				if idx := strings.Index(jline, "fid:"); idx >= 0 {
					return jline[idx+4 : idx+12]
				}
			}
		}
	}
	t.Fatal("could not extract a FID from generated journal")
	return ""
}
