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

func TestRunHledgerBalance(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing journal", args: []string{}, wantErr: "missing"},
		{name: "simple journal", args: []string{testdataPath("simple.journal")}},
		{name: "with depth flag", args: []string{"--depth", "1", testdataPath("simple.journal")}},
		{name: "with query", args: []string{testdataPath("simple.journal"), "expenses"}},
		{name: "empty journal", args: []string{testdataPath("empty.journal")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flush := captureStdout(t)
			err := runHledgerBalance(tc.args)
			out := flush()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertValidJSON(t, out)
		})
	}
}

func TestRunHledgerAccounts(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing journal", args: []string{}, wantErr: "missing"},
		{name: "flat", args: []string{testdataPath("simple.journal")}},
		{name: "tree flag", args: []string{"--tree", testdataPath("simple.journal")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flush := captureStdout(t)
			err := runHledgerAccounts(tc.args)
			out := flush()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertValidJSON(t, out)
		})
	}
}

func TestRunHledgerRegister(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing journal", args: []string{}, wantErr: "missing"},
		{name: "all transactions", args: []string{testdataPath("simple.journal")}},
		{name: "with query", args: []string{testdataPath("simple.journal"), "expenses"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flush := captureStdout(t)
			err := runHledgerRegister(tc.args)
			out := flush()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertValidJSON(t, out)
		})
	}
}

func TestRunHledgerPrintCSV(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no args", args: []string{}, wantErr: "missing"},
		{name: "missing rules", args: []string{testdataPath("import.csv")}, wantErr: "missing"},
		{name: "csv and rules", args: []string{testdataPath("import.csv"), testdataPath("import.rules")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flush := captureStdout(t)
			err := runHledgerPrintCSV(tc.args)
			out := flush()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertValidJSON(t, out)
		})
	}
}

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

func TestRunHledgerCheck(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErr    string // non-empty: expect error containing this substring
		wantAnyErr bool   // true: expect any error (message unpredictable)
		wantOutput string // non-empty: expect stdout to contain this
	}{
		{name: "missing journal", args: []string{}, wantErr: "missing"},
		{name: "valid journal", args: []string{testdataPath("simple.journal")}, wantOutput: "ok"},
		{name: "invalid journal", args: []string{testdataPath("invalid.journal")}, wantAnyErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flush := captureStdout(t)
			err := runHledgerCheck(tc.args)
			out := flush()
			switch {
			case tc.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("got error %v, want containing %q", err, tc.wantErr)
				}
			case tc.wantAnyErr:
				if err == nil {
					t.Fatal("expected error")
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.wantOutput != "" && !strings.Contains(out, tc.wantOutput) {
					t.Errorf("output %q does not contain %q", out, tc.wantOutput)
				}
			}
		})
	}
}
