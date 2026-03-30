package journal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
)

// mustHledger creates a hledger client for formatting tests, skipping if unavailable.
func mustHledger(t *testing.T) *hledger.Client {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "empty.journal")
	if err := os.WriteFile(tmp, []byte("account assets:checking\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := hledger.New("hledger", tmp)
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	return c
}

func setupJournalDir(t *testing.T, includes []string, journalFiles map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	var mainLines string
	for _, inc := range includes {
		mainLines += "include " + inc + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(mainLines), 0644); err != nil {
		t.Fatal(err)
	}
	for relPath, content := range journalFiles {
		abs := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// ---- FID tests ----

func TestMintFID(t *testing.T) {
	hexRe := regexp.MustCompile(`^[0-9a-f]{8}$`)
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "length is 8",
			check: func(t *testing.T) {
				if got := len(journal.MintFID()); got != 8 {
					t.Errorf("len(MintFID()) = %d, want 8", got)
				}
			},
		},
		{
			name: "only lowercase hex chars",
			check: func(t *testing.T) {
				if fid := journal.MintFID(); !hexRe.MatchString(fid) {
					t.Errorf("MintFID() = %q, want 8 hex chars", fid)
				}
			},
		},
		{
			name: "unique on each call",
			check: func(t *testing.T) {
				if a, b := journal.MintFID(), journal.MintFID(); a == b {
					t.Errorf("two MintFID() calls returned same value %q", a)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

// ---- Format tests ----

func TestFormatViaHledger(t *testing.T) {
	tests := []struct {
		name     string
		tx       journal.TransactionInput
		fid      string
		contains []string
	}{
		{
			name: "basic transaction",
			fid:  "bb002200",
			tx: journal.TransactionInput{
				Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				Description: "AMAZON",
				Postings: []journal.PostingInput{
					{Account: "expenses:shopping", Amount: "$45.00"},
					{Account: "assets:checking", Amount: "-$45.00"},
				},
			},
			contains: []string{"2026-01-15", "(bb002200)", "AMAZON", "expenses:shopping", "45.00"},
		},
		{
			name: "auto-balance posting",
			fid:  "aa001122",
			tx: journal.TransactionInput{
				Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
				Description: "TEST",
				Postings: []journal.PostingInput{
					{Account: "expenses:food", Amount: "$10.00"},
					{Account: "assets:checking"},
				},
			},
			contains: []string{"expenses:food", "assets:checking"},
		},
		{
			name: "fid tag preserved",
			fid:  "aa001100",
			tx: journal.TransactionInput{
				Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
				Description: "PAYROLL",
				Postings: []journal.PostingInput{
					{Account: "assets:checking", Amount: "$3500.00"},
					{Account: "income:salary"},
				},
			},
			contains: []string{"(aa001100)"},
		},
		{
			name: "output ends with newline",
			fid:  "ff001100",
			tx: journal.TransactionInput{
				Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
				Description: "TEST",
				Postings: []journal.PostingInput{
					{Account: "assets:checking", Amount: "$1.00"},
					{Account: "income:other"},
				},
			},
		},
		{
			name: "comment on separate line from fid",
			fid:  "cc003300",
			tx: journal.TransactionInput{
				Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				Description: "GROCERIES",
				Comment:     "category:food",
				Postings: []journal.PostingInput{
					{Account: "expenses:food", Amount: "$30.00"},
					{Account: "assets:checking"},
				},
			},
			contains: []string{"(cc003300)", "category:food"},
		},
	}
	c := mustHledger(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := journal.FormatViaHledger(t.Context(), c, tt.tx, tt.fid)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("output does not end with newline: %q", out)
			}
			if tt.tx.Comment != "" {
				headerLine := strings.SplitN(out, "\n", 2)[0]
				if strings.Contains(headerLine, tt.tx.Comment) {
					t.Errorf("comment %q should not appear on header line: %q", tt.tx.Comment, headerLine)
				}
				if !strings.Contains(out, "\n    ; "+tt.tx.Comment) && !strings.Contains(out, "\n; "+tt.tx.Comment) {
					t.Errorf("comment %q not found on separate indented line:\n%s", tt.tx.Comment, out)
				}
			}
		})
	}
}

// ---- File management tests ----

func TestEnsureMonthFile(t *testing.T) {
	tests := []struct {
		name    string
		year    int
		month   int
		setup   func(t *testing.T, dir string)
		check   func(t *testing.T, dir, relPath string, created bool)
	}{
		{
			name:  "creates file and directory",
			year:  2026,
			month: 1,
			check: func(t *testing.T, dir, relPath string, created bool) {
				if relPath != "2026/01.journal" {
					t.Errorf("relPath = %q, want %q", relPath, "2026/01.journal")
				}
				if !created {
					t.Error("created = false, want true")
				}
				data, err := os.ReadFile(filepath.Join(dir, relPath))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(string(data), "; float: 2026/01") {
					t.Errorf("unexpected file header: %q", string(data))
				}
			},
		},
		{
			name:  "idempotent on second call",
			year:  2026,
			month: 1,
			setup: func(t *testing.T, dir string) {
				if _, _, err := journal.EnsureMonthFile(dir, 2026, 1); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, dir, relPath string, created bool) {
				if created {
					t.Error("second call: created = true, want false")
				}
				if relPath != "2026/01.journal" {
					t.Errorf("relPath = %q, want %q", relPath, "2026/01.journal")
				}
			},
		},
		{
			name:  "creates year directory",
			year:  2026,
			month: 3,
			check: func(t *testing.T, dir, _ string, _ bool) {
				if _, err := os.Stat(filepath.Join(dir, "2026")); err != nil {
					t.Error("year directory not created")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			relPath, created, err := journal.EnsureMonthFile(dir, tt.year, tt.month)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, dir, relPath, created)
		})
	}
}

func TestUpdateMainIncludes(t *testing.T) {
	tests := []struct {
		name        string
		initial     string
		directive   string
		check       func(t *testing.T, data string)
	}{
		{
			name:      "adds directive to empty file",
			initial:   "",
			directive: "2026/01.journal",
			check: func(t *testing.T, data string) {
				if !strings.Contains(data, "include 2026/01.journal") {
					t.Errorf("directive not found: %q", data)
				}
			},
		},
		{
			name:      "idempotent on duplicate call",
			initial:   "include 2026/01.journal\n",
			directive: "2026/01.journal",
			check: func(t *testing.T, data string) {
				if count := strings.Count(data, "include 2026/01.journal"); count != 1 {
					t.Errorf("directive appears %d times, want 1:\n%s", count, data)
				}
			},
		},
		{
			name:      "preserves existing content",
			initial:   "; float main journal\ninclude accounts.journal\n",
			directive: "2026/01.journal",
			check: func(t *testing.T, data string) {
				if !strings.Contains(data, "include accounts.journal") {
					t.Error("existing content was removed")
				}
				if !strings.Contains(data, "include 2026/01.journal") {
					t.Error("new directive not added")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mainPath := filepath.Join(t.TempDir(), "main.journal")
			if tt.initial != "" {
				if err := os.WriteFile(mainPath, []byte(tt.initial), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := journal.UpdateMainIncludes(mainPath, tt.directive); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(mainPath)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, string(data))
		})
	}
}

func TestAppendTransaction(t *testing.T) {
	hexRe := regexp.MustCompile(`^[0-9a-f]{8}$`)

	tests := []struct {
		name  string
		txns  []journal.TransactionInput
		check func(t *testing.T, dir string, fids []string)
	}{
		{
			name: "single transaction written with fid",
			txns: []journal.TransactionInput{
				{
					Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					Description: "AMAZON MARKETPLACE",
					Postings: []journal.PostingInput{
						{Account: "expenses:shopping", Amount: "$45.00"},
						{Account: "assets:checking"},
					},
				},
			},
			check: func(t *testing.T, dir string, fids []string) {
				if !hexRe.MatchString(fids[0]) {
					t.Errorf("fid %q is not 8 hex chars", fids[0])
				}
				data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				if err != nil {
					t.Fatal(err)
				}
				content := string(data)
				if !strings.Contains(content, "AMAZON MARKETPLACE") {
					t.Error("transaction description not found in file")
				}
				if !strings.Contains(content, "("+fids[0]+")") {
					t.Error("fid code not found in file")
				}
				if !strings.Contains(content, "2026-01-15") {
					t.Error("ISO date not found in file")
				}
			},
		},
		{
			name: "new month file added to main.journal include",
			txns: []journal.TransactionInput{
				{
					Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					Description: "TEST",
					Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$1.00"}, {Account: "assets:checking"}},
				},
			},
			check: func(t *testing.T, dir string, _ []string) {
				data, err := os.ReadFile(filepath.Join(dir, "main.journal"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "include 2026/01.journal") {
					t.Errorf("main.journal missing include: %s", data)
				}
			},
		},
		{
			name: "transactions across months create separate files",
			txns: []journal.TransactionInput{
				{
					Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					Description: "JANUARY",
					Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$1.00"}, {Account: "assets:checking"}},
				},
				{
					Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
					Description: "FEBRUARY",
					Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$2.00"}, {Account: "assets:checking"}},
				},
			},
			check: func(t *testing.T, dir string, _ []string) {
				if _, err := os.Stat(filepath.Join(dir, "2026/01.journal")); err != nil {
					t.Error("2026/01.journal not created")
				}
				if _, err := os.Stat(filepath.Join(dir, "2026/02.journal")); err != nil {
					t.Error("2026/02.journal not created")
				}
				main, err := os.ReadFile(filepath.Join(dir, "main.journal"))
				if err != nil {
					t.Fatal(err)
				}
				s := string(main)
				if !strings.Contains(s, "include 2026/01.journal") || !strings.Contains(s, "include 2026/02.journal") {
					t.Errorf("main.journal missing includes:\n%s", s)
				}
			},
		},
		{
			name: "two transactions same month both appear in file",
			txns: []journal.TransactionInput{
				{
					Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					Description: "FIRST",
					Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$1.00"}, {Account: "assets:checking"}},
				},
				{
					Date:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
					Description: "SECOND",
					Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$2.00"}, {Account: "assets:checking"}},
				},
			},
			check: func(t *testing.T, dir string, _ []string) {
				data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				if err != nil {
					t.Fatal(err)
				}
				content := string(data)
				if !strings.Contains(content, "FIRST") || !strings.Contains(content, "SECOND") {
					t.Errorf("both transactions not in file:\n%s", content)
				}
			},
		},
	}
	c := mustHledger(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// Pre-create main.journal so UpdateMainIncludes can find it.
			if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("; float main journal\n"), 0644); err != nil {
				t.Fatal(err)
			}
			var fids []string
			for _, tx := range tt.txns {
				fid, err := journal.AppendTransaction(t.Context(), c, dir, tx)
				if err != nil {
					t.Fatal(err)
				}
				fids = append(fids, fid)
			}
			tt.check(t, dir, fids)
		})
	}
}

// ---- Migration tests ----

func TestMigrateFIDs(t *testing.T) {
	fidRe := regexp.MustCompile(`\([0-9a-f]{8}\)`)

	tests := []struct {
		name     string
		includes []string
		files    map[string]string
		wantN    int
		check    func(t *testing.T, dir string)
	}{
		{
			name:  "no main.journal returns zero",
			wantN: 0,
		},
		{
			name:     "adds fid to all untagged transactions",
			includes: []string{"2026/01.journal"},
			files: map[string]string{
				"2026/01.journal": "2026/01/05 PAYROLL\n    assets:checking  $3500.00\n    income:salary\n\n2026/01/15 AMAZON\n    expenses:shopping  $45.00\n    assets:checking\n",
			},
			wantN: 2,
			check: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				if matches := fidRe.FindAllString(string(data), -1); len(matches) != 2 {
					t.Errorf("expected 2 fid codes, found %d:\n%s", len(matches), data)
				}
			},
		},
		{
			name:     "migrates old fid tag to code field",
			includes: []string{"2026/01.journal"},
			files: map[string]string{
				"2026/01.journal": "2026/01/05 PAYROLL  ; fid:aa001100\n    assets:checking  $3500.00\n    income:salary\n",
			},
			wantN: 1,
			check: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				content := string(data)
				if !strings.Contains(content, "(aa001100)") {
					t.Errorf("expected code field (aa001100) in file:\n%s", content)
				}
				if strings.Contains(content, "fid:aa001100") {
					t.Errorf("old fid tag should have been removed:\n%s", content)
				}
			},
		},
		{
			name:     "migrates all in mixed file",
			includes: []string{"2026/01.journal"},
			files: map[string]string{
				"2026/01.journal": "2026/01/05 PAYROLL  ; fid:aa001100\n    assets:checking  $3500.00\n    income:salary\n\n2026/01/10 AMAZON  ; fid:bb002200\n    expenses:shopping  $45.00\n    assets:checking\n\n2026/01/15 WHOLE FOODS\n    expenses:food  $30.00\n    assets:checking\n",
			},
			wantN: 3,
		},
		{
			name:     "generated fid is valid hex",
			includes: []string{"2026/01.journal"},
			files: map[string]string{
				"2026/01.journal": "2026/01/15 UNTAGGED\n    expenses:misc  $1.00\n    assets:checking\n",
			},
			wantN: 1,
			check: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				if m := fidRe.FindString(string(data)); m == "" {
					t.Errorf("no valid fid code found:\n%s", data)
				}
			},
		},
		{
			name:     "posting lines are not modified",
			includes: []string{"2026/01.journal"},
			files: map[string]string{
				"2026/01.journal": "2026/01/15 UNTAGGED\n    expenses:misc  $1.00\n    assets:checking\n",
			},
			wantN: 1,
			check: func(t *testing.T, dir string) {
				data, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				s := string(data)
				if !strings.Contains(s, "expenses:misc  $1.00") {
					t.Errorf("posting line was modified:\n%s", s)
				}
				if !strings.Contains(s, "assets:checking") {
					t.Errorf("auto-balance posting removed:\n%s", s)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dir string
			if tt.includes == nil && tt.files == nil {
				dir = t.TempDir()
			} else {
				dir = setupJournalDir(t, tt.includes, tt.files)
			}
			n, err := journal.MigrateFIDs(dir)
			if err != nil {
				t.Fatal(err)
			}
			if n != tt.wantN {
				t.Errorf("MigrateFIDs() = %d, want %d", n, tt.wantN)
			}
			if tt.check != nil {
				tt.check(t, dir)
			}
		})
	}
}
