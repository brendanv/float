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

func TestMintFID_Length(t *testing.T) {
	if got := len(journal.MintFID()); got != 8 {
		t.Errorf("len(MintFID()) = %d, want 8", got)
	}
}

func TestMintFID_HexChars(t *testing.T) {
	hexRe := regexp.MustCompile(`^[0-9a-f]{8}$`)
	fid := journal.MintFID()
	if !hexRe.MatchString(fid) {
		t.Errorf("MintFID() = %q, want 8 hex chars", fid)
	}
}

func TestMintFID_Unique(t *testing.T) {
	a, b := journal.MintFID(), journal.MintFID()
	if a == b {
		t.Errorf("two MintFID() calls returned same value %q", a)
	}
}

// ---- Format tests ----

func TestFormatViaHledger_Basic(t *testing.T) {
	c := mustHledger(t)
	tx := journal.TransactionInput{
		Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Description: "AMAZON",
		Postings: []journal.PostingInput{
			{Account: "expenses:shopping", Amount: "$45.00"},
			{Account: "assets:checking", Amount: "-$45.00"},
		},
	}
	out, err := journal.FormatViaHledger(t.Context(), c, tx, "bb002200")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2026-01-15 AMAZON") {
		t.Errorf("missing date/desc in output:\n%s", out)
	}
	if !strings.Contains(out, "fid:bb002200") {
		t.Errorf("missing fid tag in output:\n%s", out)
	}
	if !strings.Contains(out, "expenses:shopping") {
		t.Errorf("missing posting account in output:\n%s", out)
	}
	if !strings.Contains(out, "45.00") {
		t.Errorf("missing amount in output:\n%s", out)
	}
}

func TestFormatViaHledger_AutoBalance(t *testing.T) {
	c := mustHledger(t)
	tx := journal.TransactionInput{
		Date:        time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Description: "TEST",
		Postings: []journal.PostingInput{
			{Account: "expenses:food", Amount: "$10.00"},
			{Account: "assets:checking"},
		},
	}
	out, err := journal.FormatViaHledger(t.Context(), c, tx, "aa001122")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "expenses:food") || !strings.Contains(out, "assets:checking") {
		t.Errorf("missing postings in output:\n%s", out)
	}
}

func TestFormatViaHledger_FIDPreserved(t *testing.T) {
	c := mustHledger(t)
	tx := journal.TransactionInput{
		Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Description: "PAYROLL",
		Postings: []journal.PostingInput{
			{Account: "assets:checking", Amount: "$3500.00"},
			{Account: "income:salary"},
		},
	}
	out, err := journal.FormatViaHledger(t.Context(), c, tx, "aa001100")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fid:aa001100") {
		t.Errorf("fid not preserved in output:\n%s", out)
	}
}

func TestFormatViaHledger_TrailingNewline(t *testing.T) {
	c := mustHledger(t)
	tx := journal.TransactionInput{
		Date:        time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Description: "TEST",
		Postings: []journal.PostingInput{
			{Account: "assets:checking", Amount: "$1.00"},
			{Account: "income:other"},
		},
	}
	out, err := journal.FormatViaHledger(t.Context(), c, tx, "ff001100")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output does not end with newline: %q", out)
	}
}

// ---- File management tests ----

func TestEnsureMonthFile_Creates(t *testing.T) {
	dir := t.TempDir()
	relPath, created, err := journal.EnsureMonthFile(dir, 2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	if relPath != "2026/01.journal" {
		t.Errorf("relPath = %q, want %q", relPath, "2026/01.journal")
	}
	if !created {
		t.Error("created = false, want true")
	}
	absPath := filepath.Join(dir, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "; float: 2026/01") {
		t.Errorf("unexpected file header: %q", string(data))
	}
}

func TestEnsureMonthFile_Idempotent(t *testing.T) {
	dir := t.TempDir()
	rel1, _, err := journal.EnsureMonthFile(dir, 2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	rel2, created, err := journal.EnsureMonthFile(dir, 2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second call: created = true, want false")
	}
	if rel1 != rel2 {
		t.Errorf("relPath changed: %q vs %q", rel1, rel2)
	}
}

func TestEnsureMonthFile_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := journal.EnsureMonthFile(dir, 2026, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026")); err != nil {
		t.Error("year directory not created")
	}
}

func TestUpdateMainIncludes_Adds(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.journal")
	if err := journal.UpdateMainIncludes(mainPath, "2026/01.journal"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "include 2026/01.journal") {
		t.Errorf("directive not found: %q", string(data))
	}
}

func TestUpdateMainIncludes_Idempotent(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.journal")
	if err := journal.UpdateMainIncludes(mainPath, "2026/01.journal"); err != nil {
		t.Fatal(err)
	}
	if err := journal.UpdateMainIncludes(mainPath, "2026/01.journal"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(string(data), "include 2026/01.journal")
	if count != 1 {
		t.Errorf("directive appears %d times, want 1:\n%s", count, data)
	}
}

func TestUpdateMainIncludes_Preserves(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.journal")
	if err := os.WriteFile(mainPath, []byte("; float main journal\ninclude accounts.journal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := journal.UpdateMainIncludes(mainPath, "2026/01.journal"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "include accounts.journal") {
		t.Error("existing content was removed")
	}
	if !strings.Contains(string(data), "include 2026/01.journal") {
		t.Error("new directive not added")
	}
}

func TestAppendTransaction_Basic(t *testing.T) {
	c := mustHledger(t)
	dir := t.TempDir()
	tx := journal.TransactionInput{
		Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Description: "AMAZON MARKETPLACE",
		Postings: []journal.PostingInput{
			{Account: "expenses:shopping", Amount: "$45.00"},
			{Account: "assets:checking"},
		},
	}
	fid, err := journal.AppendTransaction(t.Context(), c, dir, tx)
	if err != nil {
		t.Fatal(err)
	}
	hexRe := regexp.MustCompile(`^[0-9a-f]{8}$`)
	if !hexRe.MatchString(fid) {
		t.Errorf("fid %q is not 8 hex chars", fid)
	}
	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "AMAZON MARKETPLACE") {
		t.Error("transaction description not found in file")
	}
	if !strings.Contains(content, "fid:"+fid) {
		t.Error("fid tag not found in file")
	}
	if !strings.Contains(content, "2026-01-15") {
		t.Error("ISO date not found in file")
	}
}

func TestAppendTransaction_UpdatesMain(t *testing.T) {
	c := mustHledger(t)
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.journal")
	if err := os.WriteFile(mainPath, []byte("; float main journal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tx := journal.TransactionInput{
		Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Description: "TEST",
		Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$1.00"}, {Account: "assets:checking"}},
	}
	if _, err := journal.AppendTransaction(t.Context(), c, dir, tx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "include 2026/01.journal") {
		t.Errorf("main.journal missing include: %s", data)
	}
}

func TestAppendTransaction_MultipleMonths(t *testing.T) {
	c := mustHledger(t)
	dir := t.TempDir()
	txJan := journal.TransactionInput{
		Date:        time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Description: "JANUARY",
		Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$1.00"}, {Account: "assets:checking"}},
	}
	txFeb := journal.TransactionInput{
		Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Description: "FEBRUARY",
		Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$2.00"}, {Account: "assets:checking"}},
	}
	if _, err := journal.AppendTransaction(t.Context(), c, dir, txJan); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendTransaction(t.Context(), c, dir, txFeb); err != nil {
		t.Fatal(err)
	}

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
	if !strings.Contains(string(main), "include 2026/01.journal") || !strings.Contains(string(main), "include 2026/02.journal") {
		t.Errorf("main.journal missing includes:\n%s", main)
	}
}

func TestAppendTransaction_SameMonth(t *testing.T) {
	c := mustHledger(t)
	dir := t.TempDir()
	tx1 := journal.TransactionInput{
		Date:        time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		Description: "FIRST",
		Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$1.00"}, {Account: "assets:checking"}},
	}
	tx2 := journal.TransactionInput{
		Date:        time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC),
		Description: "SECOND",
		Postings:    []journal.PostingInput{{Account: "expenses:misc", Amount: "$2.00"}, {Account: "assets:checking"}},
	}
	if _, err := journal.AppendTransaction(t.Context(), c, dir, tx1); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.AppendTransaction(t.Context(), c, dir, tx2); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "FIRST") || !strings.Contains(content, "SECOND") {
		t.Errorf("both transactions not in file:\n%s", content)
	}
}

// ---- Migration tests ----

func TestMigrateFIDs_NoMainJournal(t *testing.T) {
	dir := t.TempDir()
	n, err := journal.MigrateFIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("got %d, want 0", n)
	}
}

func TestMigrateFIDs_AddsToUntagged(t *testing.T) {
	journalContent := `2026/01/05 PAYROLL
    assets:checking  $3500.00
    income:salary

2026/01/15 AMAZON
    expenses:shopping  $45.00
    assets:checking
`
	dir := setupJournalDir(t, []string{"2026/01.journal"}, map[string]string{"2026/01.journal": journalContent})

	n, err := journal.MigrateFIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("got %d modified, want 2", n)
	}
	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	fidRe := regexp.MustCompile(`fid:[0-9a-f]{8}`)
	matches := fidRe.FindAllString(string(data), -1)
	if len(matches) != 2 {
		t.Errorf("expected 2 fid tags, found %d:\n%s", len(matches), data)
	}
}

func TestMigrateFIDs_PreservesExisting(t *testing.T) {
	journalContent := "2026/01/05 PAYROLL  ; fid:aa001100\n    assets:checking  $3500.00\n    income:salary\n"
	dir := setupJournalDir(t, []string{"2026/01.journal"}, map[string]string{"2026/01.journal": journalContent})

	n, err := journal.MigrateFIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("got %d modified, want 0", n)
	}
	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != journalContent {
		t.Errorf("file was modified unexpectedly:\n%s", data)
	}
}

func TestMigrateFIDs_Mixed(t *testing.T) {
	journalContent := `2026/01/05 PAYROLL  ; fid:aa001100
    assets:checking  $3500.00
    income:salary

2026/01/10 AMAZON  ; fid:bb002200
    expenses:shopping  $45.00
    assets:checking

2026/01/15 WHOLE FOODS
    expenses:food  $30.00
    assets:checking
`
	dir := setupJournalDir(t, []string{"2026/01.journal"}, map[string]string{"2026/01.journal": journalContent})

	n, err := journal.MigrateFIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("got %d modified, want 1", n)
	}
}

func TestMigrateFIDs_ValidFidFormat(t *testing.T) {
	journalContent := "2026/01/15 UNTAGGED\n    expenses:misc  $1.00\n    assets:checking\n"
	dir := setupJournalDir(t, []string{"2026/01.journal"}, map[string]string{"2026/01.journal": journalContent})

	if _, err := journal.MigrateFIDs(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	hexRe := regexp.MustCompile(`fid:([0-9a-f]{8})`)
	if m := hexRe.FindString(string(data)); m == "" {
		t.Errorf("no valid fid tag found:\n%s", data)
	}
}

func TestMigrateFIDs_PreservesPostings(t *testing.T) {
	journalContent := "2026/01/15 UNTAGGED\n    expenses:misc  $1.00\n    assets:checking\n"
	dir := setupJournalDir(t, []string{"2026/01.journal"}, map[string]string{"2026/01.journal": journalContent})

	if _, err := journal.MigrateFIDs(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "expenses:misc  $1.00") {
		t.Errorf("posting line was modified:\n%s", data)
	}
	if !strings.Contains(string(data), "assets:checking") {
		t.Errorf("auto-balance posting removed:\n%s", data)
	}
}
