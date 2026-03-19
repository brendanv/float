package testgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/testgen"
)

// mustHledger creates a hledger client for validation, skipping if unavailable.
func mustHledger(t *testing.T, journal string) *hledger.Client {
	t.Helper()
	c, err := hledger.New("hledger", journal)
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	return c
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		name  string
		opts  testgen.Options
		check func(t *testing.T, out string)
	}{
		{
			name: "defaults produce non-empty output",
			opts: testgen.Options{Seed: 42, WithFIDs: true},
			check: func(t *testing.T, out string) {
				if strings.TrimSpace(out) == "" {
					t.Error("Generate() returned empty output")
				}
			},
		},
		{
			name: "deterministic with same seed",
			opts: testgen.Options{Seed: 99, NumTxns: 5, WithFIDs: true},
			check: func(t *testing.T, out string) {
				second := testgen.Generate(testgen.Options{Seed: 99, NumTxns: 5, WithFIDs: true})
				if out != second {
					t.Error("two calls with the same seed produced different output")
				}
			},
		},
		{
			name: "different seeds produce different output",
			opts: testgen.Options{Seed: 1, NumTxns: 5, WithFIDs: true},
			check: func(t *testing.T, out string) {
				other := testgen.Generate(testgen.Options{Seed: 2, NumTxns: 5, WithFIDs: true})
				if out == other {
					t.Error("different seeds produced identical output")
				}
			},
		},
		{
			name: "fid tags present when WithFIDs true",
			opts: testgen.Options{Seed: 1, NumTxns: 3, WithFIDs: true},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "fid:") {
					t.Error("output missing fid: tags")
				}
			},
		},
		{
			name: "fid tags absent when WithFIDs false",
			opts: testgen.Options{Seed: 1, NumTxns: 3, WithFIDs: false},
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "fid:") {
					t.Error("output contains fid: tags when WithFIDs is false")
				}
			},
		},
		{
			name: "account declarations present for custom accounts",
			opts: testgen.Options{
				Seed:     1,
				Accounts: []string{"assets:bank", "expenses:misc"},
				NumTxns:  2,
				WithFIDs: true,
			},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "account assets:bank") {
					t.Error("missing account declaration for assets:bank")
				}
				if !strings.Contains(out, "account expenses:misc") {
					t.Error("missing account declaration for expenses:misc")
				}
			},
		},
		{
			name: "dates fall within range",
			opts: testgen.Options{
				Seed:      42,
				NumTxns:   10,
				StartDate: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC),
				WithFIDs:  true,
			},
			check: func(t *testing.T, out string) {
				if !strings.Contains(out, "2025-06-") {
					t.Error("output does not contain dates in 2025-06")
				}
				if strings.Contains(out, "2025-07-") {
					t.Error("output contains dates outside the requested range")
				}
			},
		},
		{
			name: "requested transaction count is produced",
			opts: testgen.Options{Seed: 7, NumTxns: 15, WithFIDs: true},
			check: func(t *testing.T, out string) {
				// Each transaction header line starts with a date (YYYY-MM-DD)
				count := 0
				for _, line := range strings.Split(out, "\n") {
					if len(line) >= 10 && line[4] == '-' && line[7] == '-' && line[0] >= '2' {
						count++
					}
				}
				if count != 15 {
					t.Errorf("expected 15 transaction headers, got %d", count)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := testgen.Generate(tc.opts)
			tc.check(t, out)
		})
	}
}

func TestGenerateProducesValidJournal(t *testing.T) {
	out := testgen.Generate(testgen.Options{Seed: 42, NumTxns: 10, WithFIDs: true})

	tmp := filepath.Join(t.TempDir(), "gen.journal")
	if err := os.WriteFile(tmp, []byte(out), 0644); err != nil {
		t.Fatal(err)
	}

	c := mustHledger(t, tmp)
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("generated journal failed hledger check: %v\n\n%s", err, out)
	}
}

func TestGenerateDataDir(t *testing.T) {
	tests := []struct {
		name  string
		opts  testgen.Options
		check func(t *testing.T, dir string)
	}{
		{
			name: "main.journal is created",
			opts: testgen.Options{Seed: 1, NumTxns: 5, WithFIDs: true},
			check: func(t *testing.T, dir string) {
				if _, err := os.Stat(filepath.Join(dir, "main.journal")); err != nil {
					t.Errorf("main.journal not created: %v", err)
				}
			},
		},
		{
			name: "main.journal contains include directives",
			opts: testgen.Options{Seed: 1, NumTxns: 5, WithFIDs: true},
			check: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "main.journal"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "include ") {
					t.Error("main.journal has no include directives")
				}
			},
		},
		{
			name: "month journal files are created",
			opts: testgen.Options{
				Seed:      1,
				NumTxns:   10,
				StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndDate:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
				WithFIDs:  true,
			},
			check: func(t *testing.T, dir string) {
				// With 10 txns spread over 3 months, at least one month file must exist
				var found int
				for _, f := range []string{"2026/01.journal", "2026/02.journal", "2026/03.journal"} {
					if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
						found++
					}
				}
				if found == 0 {
					t.Error("no month journal files were created")
				}
			},
		},
		{
			name: "deterministic layout with same seed",
			opts: testgen.Options{Seed: 77, NumTxns: 5, WithFIDs: true},
			check: func(t *testing.T, dir string) {
				data1, err := os.ReadFile(filepath.Join(dir, "main.journal"))
				if err != nil {
					t.Fatal(err)
				}
				// Generate a second dir and compare main.journal
				dir2 := testgen.GenerateDataDir(t, testgen.Options{Seed: 77, NumTxns: 5, WithFIDs: true})
				data2, err := os.ReadFile(filepath.Join(dir2, "main.journal"))
				if err != nil {
					t.Fatal(err)
				}
				if string(data1) != string(data2) {
					t.Error("same seed produced different main.journal content")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testgen.GenerateDataDir(t, tc.opts)
			tc.check(t, dir)
		})
	}
}

func TestGenerateDataDirProducesValidJournal(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{
		Seed:     42,
		NumTxns:  20,
		WithFIDs: true,
	})
	mainJournal := filepath.Join(dir, "main.journal")

	c := mustHledger(t, mainJournal)
	if err := c.Check(t.Context()); err != nil {
		t.Errorf("generated data dir failed hledger check: %v", err)
	}
}
