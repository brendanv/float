// Package testgen generates synthetic hledger journal content for use in tests
// and developer tooling. Journals are deterministic when a non-zero Seed is
// provided, making them suitable for reproducible test scenarios.
//
// Generated journals use the same file layout as real float data directories
// (main.journal + YYYY/MM.journal files) and produce valid hledger syntax.
package testgen

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// defaultAccounts is used when Options.Accounts is empty.
var defaultAccounts = []string{
	"assets:checking",
	"assets:savings",
	"expenses:food",
	"expenses:shopping",
	"expenses:utilities",
	"expenses:transport",
	"income:salary",
	"income:other",
}

// Options controls what testgen produces.
type Options struct {
	// Accounts to use for postings. Must have at least 2 entries.
	// Defaults to defaultAccounts if empty.
	Accounts []string

	// NumTxns is the total number of transactions to generate.
	// Defaults to 10.
	NumTxns int

	// StartDate is the earliest transaction date (inclusive).
	// Defaults to 2026-01-01.
	StartDate time.Time

	// EndDate is the latest transaction date (inclusive).
	// Defaults to 2026-03-31.
	EndDate time.Time

	// Seed controls the random source. 0 means use a random seed (non-deterministic).
	// Any non-zero value produces identical output on each call with the same options.
	Seed int64

	// WithFIDs attaches (XXXXXXXX) code fields to each transaction header.
	// Enabled by default.
	WithFIDs bool
}

func (o *Options) withDefaults() Options {
	out := *o
	if len(out.Accounts) == 0 {
		out.Accounts = defaultAccounts
	}
	if out.NumTxns == 0 {
		out.NumTxns = 10
	}
	if out.StartDate.IsZero() {
		out.StartDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if out.EndDate.IsZero() {
		out.EndDate = time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	}
	return out
}

// Generate returns a complete journal file as a string, including account
// declarations and transactions. The output is valid hledger journal syntax
// and can be written directly to a .journal file.
func Generate(opts Options) string {
	o := opts.withDefaults()

	var rng *rand.Rand
	if o.Seed != 0 {
		rng = rand.New(rand.NewSource(o.Seed)) //nolint:gosec // deterministic test data
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // test data only
	}

	var b strings.Builder

	// Account declarations
	seen := make(map[string]bool)
	for _, a := range o.Accounts {
		if !seen[a] {
			fmt.Fprintf(&b, "account %s\n", a)
			seen[a] = true
		}
	}
	b.WriteString("\n")

	spanDays := int(o.EndDate.Sub(o.StartDate).Hours()/24) + 1
	if spanDays < 1 {
		spanDays = 1
	}

	descriptions := []string{
		"PAYROLL DIRECT DEPOSIT",
		"AMAZON MARKETPLACE",
		"WHOLE FOODS",
		"NETFLIX",
		"ELECTRIC BILL",
		"GAS STATION",
		"COFFEE SHOP",
		"GROCERY STORE",
		"ONLINE TRANSFER",
		"ATM WITHDRAWAL",
	}

	for i := range o.NumTxns {
		// Spread transactions across the date range
		dayOffset := 0
		if spanDays > 1 {
			dayOffset = (i * spanDays) / o.NumTxns
		}
		date := o.StartDate.AddDate(0, 0, dayOffset)

		desc := descriptions[rng.Intn(len(descriptions))]

		// Pick two distinct accounts
		acctIdx := rng.Intn(len(o.Accounts))
		acct1 := o.Accounts[acctIdx]
		acct2Idx := (acctIdx + 1 + rng.Intn(len(o.Accounts)-1)) % len(o.Accounts)
		acct2 := o.Accounts[acct2Idx]

		// Random amount: $1.00–$500.00
		cents := 100 + rng.Intn(49900)
		amount := fmt.Sprintf("$%d.%02d", cents/100, cents%100)

		if o.WithFIDs {
			fid := randomFID(rng)
			fmt.Fprintf(&b, "%s (%s) %s\n", date.Format("2006-01-02"), fid, desc)
		} else {
			fmt.Fprintf(&b, "%s %s\n", date.Format("2006-01-02"), desc)
		}
		fmt.Fprintf(&b, "    %s  %s\n", acct1, amount)
		fmt.Fprintf(&b, "    %s\n", acct2)
		b.WriteString("\n")
	}

	return b.String()
}

// randomFID returns an 8-char lowercase hex string from the given rng.
func randomFID(rng *rand.Rand) string {
	return fmt.Sprintf("%08x", rng.Uint32())
}

// GenerateDataDir writes a complete data directory layout to a temp directory
// and returns its path. The caller is responsible for cleanup (t.TempDir()
// handles this automatically when called with a *testing.T).
//
// The layout matches float's real data directory structure:
//
//	<dir>/main.journal        — include directives only
//	<dir>/YYYY/MM.journal     — transactions grouped by month
//
// Transactions are distributed across months according to their dates.
func GenerateDataDir(t testing.TB, opts Options) string {
	t.Helper()
	o := opts.withDefaults()

	var rng *rand.Rand
	if o.Seed != 0 {
		rng = rand.New(rand.NewSource(o.Seed)) //nolint:gosec // deterministic test data
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // test data only
	}

	dir := t.TempDir()

	spanDays := int(o.EndDate.Sub(o.StartDate).Hours()/24) + 1
	if spanDays < 1 {
		spanDays = 1
	}

	descriptions := []string{
		"PAYROLL DIRECT DEPOSIT",
		"AMAZON MARKETPLACE",
		"WHOLE FOODS",
		"NETFLIX",
		"ELECTRIC BILL",
		"GAS STATION",
		"COFFEE SHOP",
		"GROCERY STORE",
		"ONLINE TRANSFER",
		"ATM WITHDRAWAL",
	}

	// Collect transactions per month key "YYYY/MM"
	type txn struct {
		date   time.Time
		text   string
	}
	monthTxns := make(map[string][]txn)
	var monthOrder []string
	monthSeen := make(map[string]bool)

	for i := range o.NumTxns {
		dayOffset := 0
		if spanDays > 1 {
			dayOffset = (i * spanDays) / o.NumTxns
		}
		date := o.StartDate.AddDate(0, 0, dayOffset)

		desc := descriptions[rng.Intn(len(descriptions))]

		acctIdx := rng.Intn(len(o.Accounts))
		acct1 := o.Accounts[acctIdx]
		acct2Idx := (acctIdx + 1 + rng.Intn(len(o.Accounts)-1)) % len(o.Accounts)
		acct2 := o.Accounts[acct2Idx]

		cents := 100 + rng.Intn(49900)
		amount := fmt.Sprintf("$%d.%02d", cents/100, cents%100)

		var sb strings.Builder
		if o.WithFIDs {
			fid := randomFID(rng)
			fmt.Fprintf(&sb, "%s (%s) %s\n", date.Format("2006-01-02"), fid, desc)
		} else {
			fmt.Fprintf(&sb, "%s %s\n", date.Format("2006-01-02"), desc)
		}
		fmt.Fprintf(&sb, "    %s  %s\n", acct1, amount)
		fmt.Fprintf(&sb, "    %s\n", acct2)
		sb.WriteString("\n")

		key := date.Format("2006/01")
		if !monthSeen[key] {
			monthOrder = append(monthOrder, key)
			monthSeen[key] = true
		}
		monthTxns[key] = append(monthTxns[key], txn{date: date, text: sb.String()})
	}

	// Write month files and collect include paths
	var includes []string
	for _, key := range monthOrder {
		relPath := key + ".journal"
		absPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("testgen: mkdir %s: %v", filepath.Dir(absPath), err)
		}

		var fb strings.Builder
		fmt.Fprintf(&fb, "; float: %s\n", key)
		for _, tx := range monthTxns[key] {
			fb.WriteString(tx.text)
		}
		if err := os.WriteFile(absPath, []byte(fb.String()), 0644); err != nil {
			t.Fatalf("testgen: write %s: %v", absPath, err)
		}
		includes = append(includes, relPath)
	}

	// Write main.journal with account declarations and include directives
	var mb strings.Builder
	mb.WriteString("; float main journal\n")
	for _, a := range o.Accounts {
		fmt.Fprintf(&mb, "account %s\n", a)
	}
	mb.WriteString("\n")
	for _, inc := range includes {
		fmt.Fprintf(&mb, "include %s\n", inc)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(mb.String()), 0644); err != nil {
		t.Fatalf("testgen: write main.journal: %v", err)
	}

	return dir
}
