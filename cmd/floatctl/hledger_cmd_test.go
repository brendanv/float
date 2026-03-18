package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testdataDir is the path to hledger test fixtures relative to this package.
const testdataDir = "../../internal/hledger/testdata"

func testdataPath(name string) string {
	return filepath.Join(testdataDir, name)
}

// captureStdout redirects os.Stdout to a pipe and returns a function that
// closes the pipe, restores os.Stdout, and returns the captured output.
// A t.Cleanup backup ensures stdout is restored even if the caller forgets.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w

	var restored bool
	restore := func() string {
		if restored {
			return ""
		}
		restored = true
		w.Close()
		os.Stdout = orig
		out, _ := io.ReadAll(r)
		r.Close()
		return string(out)
	}
	t.Cleanup(func() { restore() })
	return restore
}

// assertValidJSON fails the test if s is not valid JSON.
func assertValidJSON(t *testing.T, s string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Errorf("output is not valid JSON: %v\noutput: %s", err, s)
	}
}

// --- hledger balance ---

func TestRunHledgerBalance_MissingJournal(t *testing.T) {
	err := runHledgerBalance([]string{})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-journal error, got %v", err)
	}
}

func TestRunHledgerBalance(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerBalance([]string{testdataPath("simple.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

func TestRunHledgerBalance_WithDepth(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerBalance([]string{"--depth", "1", testdataPath("simple.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

func TestRunHledgerBalance_WithQuery(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerBalance([]string{testdataPath("simple.journal"), "expenses"})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

func TestRunHledgerBalance_EmptyJournal(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerBalance([]string{testdataPath("empty.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

// --- hledger accounts ---

func TestRunHledgerAccounts_MissingJournal(t *testing.T) {
	err := runHledgerAccounts([]string{})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-journal error, got %v", err)
	}
}

func TestRunHledgerAccounts_Flat(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerAccounts([]string{testdataPath("simple.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

func TestRunHledgerAccounts_Tree(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerAccounts([]string{"--tree", testdataPath("simple.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

// --- hledger register ---

func TestRunHledgerRegister_MissingJournal(t *testing.T) {
	err := runHledgerRegister([]string{})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-journal error, got %v", err)
	}
}

func TestRunHledgerRegister(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerRegister([]string{testdataPath("simple.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

func TestRunHledgerRegister_WithQuery(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerRegister([]string{testdataPath("simple.journal"), "expenses"})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

// --- hledger print-csv ---

func TestRunHledgerPrintCSV_NoArgs(t *testing.T) {
	err := runHledgerPrintCSV([]string{})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-args error, got %v", err)
	}
}

func TestRunHledgerPrintCSV_MissingRules(t *testing.T) {
	// Only csv provided, rules missing.
	err := runHledgerPrintCSV([]string{testdataPath("import.csv")})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-args error, got %v", err)
	}
}

func TestRunHledgerPrintCSV(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerPrintCSV([]string{testdataPath("import.csv"), testdataPath("import.rules")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertValidJSON(t, out)
}

// --- hledger version ---

func TestRunHledgerVersion(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerVersion([]string{})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Version() strips the "hledger" prefix and returns just the semver string.
	if strings.TrimSpace(out) == "" {
		t.Errorf("expected non-empty version output, got: %q", out)
	}
}

// --- hledger check ---

func TestRunHledgerCheck_MissingJournal(t *testing.T) {
	err := runHledgerCheck([]string{})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-journal error, got %v", err)
	}
}

func TestRunHledgerCheck_Valid(t *testing.T) {
	flush := captureStdout(t)
	err := runHledgerCheck([]string{testdataPath("simple.journal")})
	out := flush()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("expected 'ok' in output, got: %s", out)
	}
}

func TestRunHledgerCheck_Invalid(t *testing.T) {
	captureStdout(t) // suppress any stdout; invalid journal returns error without printing
	err := runHledgerCheck([]string{testdataPath("invalid.journal")})
	if err == nil {
		t.Fatal("expected error for invalid journal")
	}
}
