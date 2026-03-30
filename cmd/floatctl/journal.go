package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/txlock"
)

func init() {
	register(
		&Command{
			Group:    "journal",
			Name:     "add",
			Synopsis: "Add a new transaction directly to the journal via txlock",
			Run:      runJournalAdd,
		},
		&Command{
			Group:    "journal",
			Name:     "delete",
			Synopsis: "Delete a transaction by fid tag via txlock",
			Run:      runJournalDelete,
		},
		&Command{
			Group:    "journal",
			Name:     "import",
			Synopsis: "Preview and import a CSV file using a bank profile's rules",
			Run:      runJournalImport,
		},
		&Command{
			Group:    "journal",
			Name:     "verify",
			Synopsis: "Run hledger check on the full data directory; report all errors",
			Run:      runJournalVerify,
		},
		&Command{
			Group:    "journal",
			Name:     "migrate-fids",
			Synopsis: "Scan all transactions and add fid tags to any that lack them",
			Run:      runJournalMigrateFIDs,
		},
		&Command{
			Group:    "journal",
			Name:     "list-files",
			Synopsis: "List all .journal files found under the data directory",
			Run:      runJournalListFiles,
		},
		&Command{
			Group:    "journal",
			Name:     "lookup",
			Synopsis: "Look up a transaction by fid tag; print as JSON",
			Run:      runJournalLookup,
		},
		&Command{
			Group:    "journal",
			Name:     "stats",
			Synopsis: "Show journal statistics: file count, transaction count, date range, account count",
			Run:      runJournalStats,
		},
		&Command{
			Group:    "journal",
			Name:     "audit",
			Synopsis: "Audit journal integrity: check includes exist, FIDs are unique, no orphaned files",
			Run:      runJournalAudit,
		},
	)
}

func runJournalVerify(args []string) error {
	fs := flag.NewFlagSet("journal verify", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal verify <data-dir>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := fs.Arg(0)
	mainJournal := filepath.Join(dataDir, "main.journal")

	c, err := hledger.New("hledger", mainJournal)
	if err != nil {
		return err
	}
	if err := c.Check(context.Background()); err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func runJournalMigrateFIDs(args []string) error {
	fs := flag.NewFlagSet("journal migrate-fids", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal migrate-fids <data-dir>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := fs.Arg(0)

	n, err := journal.MigrateFIDs(dataDir)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Println("all transactions already have fid tags")
	} else {
		fmt.Printf("added fid tags to %d transaction(s)\n", n)
	}
	return nil
}

func runJournalListFiles(args []string) error {
	fset := flag.NewFlagSet("journal list-files", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal list-files <data-dir>")
	}
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := fset.Arg(0)

	return filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".journal" {
			rel, relErr := filepath.Rel(dataDir, path)
			if relErr != nil {
				rel = path
			}
			fmt.Println(rel)
		}
		return nil
	})
}

func runJournalLookup(args []string) error {
	fset := flag.NewFlagSet("journal lookup", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal lookup <data-dir> <fid>")
	}
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() < 2 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> and/or <fid> argument")
	}
	dataDir := fset.Arg(0)
	fid := fset.Arg(1)
	mainJournal := filepath.Join(dataDir, "main.journal")

	c, err := hledgerClient(mainJournal)
	if err != nil {
		return err
	}

	txns, err := c.Transactions(context.Background(), "code:"+fid)
	if err != nil {
		return err
	}
	switch len(txns) {
	case 0:
		return fmt.Errorf("no transaction found with fid %q", fid)
	case 1:
		return printJSON(txns[0])
	default:
		return fmt.Errorf("fid %q matched %d transactions (corrupt journal — run audit)", fid, len(txns))
	}
}

// journalStatsResult is the JSON output of `journal stats`.
type journalStatsResult struct {
	JournalFiles int    `json:"journal_files"`
	Transactions int    `json:"transactions"`
	FirstDate    string `json:"first_date,omitempty"`
	LastDate     string `json:"last_date,omitempty"`
	Accounts     int    `json:"accounts"`
	TotalBytes   int64  `json:"total_size_bytes"`
}

func runJournalStats(args []string) error {
	fset := flag.NewFlagSet("journal stats", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal stats <data-dir>")
	}
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := fset.Arg(0)
	mainJournal := filepath.Join(dataDir, "main.journal")

	c, err := hledgerClient(mainJournal)
	if err != nil {
		return err
	}
	ctx := context.Background()

	rows, err := c.Register(ctx)
	if err != nil {
		return fmt.Errorf("journal stats: register: %w", err)
	}

	accounts, err := c.Accounts(ctx, false)
	if err != nil {
		return fmt.Errorf("journal stats: accounts: %w", err)
	}

	// RegisterRow.Date is non-nil only for the first posting of each transaction.
	var first, last string
	var txnCount int
	for _, r := range rows {
		if r.Date == nil {
			continue
		}
		txnCount++
		d := *r.Date
		if first == "" || d < first {
			first = d
		}
		if last == "" || d > last {
			last = d
		}
	}

	var totalBytes int64
	var fileCount int
	_ = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(path) == ".journal" {
			fileCount++
			if info, infoErr := d.Info(); infoErr == nil {
				totalBytes += info.Size()
			}
		}
		return nil
	})

	result := journalStatsResult{
		JournalFiles: fileCount,
		Transactions: txnCount,
		FirstDate:    first,
		LastDate:     last,
		Accounts:     len(accounts),
		TotalBytes:   totalBytes,
	}
	return printJSON(result)
}

// auditResult is the JSON output of `journal audit`.
type auditResult struct {
	MissingIncludes  []string            `json:"missing_includes"`
	DuplicateFIDs    map[string][]string `json:"duplicate_fids"`
	UnincludedFiles  []string            `json:"unincluded_files"`
	OK               bool                `json:"ok"`
}

var journalIncludeRe = regexp.MustCompile(`^include\s+(.+)`)
var journalFIDRe = regexp.MustCompile(`\(([0-9a-f]{8})\)`)

func runJournalAudit(args []string) error {
	fset := flag.NewFlagSet("journal audit", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal audit <data-dir>")
	}
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := fset.Arg(0)
	mainPath := filepath.Join(dataDir, "main.journal")

	mainData, err := os.ReadFile(mainPath)
	if err != nil {
		return fmt.Errorf("journal audit: read main.journal: %w", err)
	}

	// Collect include directives from main.journal
	var includedRels []string
	for _, line := range strings.Split(string(mainData), "\n") {
		if m := journalIncludeRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			includedRels = append(includedRels, strings.TrimSpace(m[1]))
		}
	}

	// Check which included files are missing
	var missingIncludes []string
	includedSet := make(map[string]bool)
	for _, rel := range includedRels {
		abs := filepath.Join(dataDir, rel)
		includedSet[abs] = true
		if _, statErr := os.Stat(abs); os.IsNotExist(statErr) {
			missingIncludes = append(missingIncludes, rel)
		}
	}

	// Scan included files for FID tags, detect duplicates
	// fidLocations maps fid → list of "file:lineN" strings
	fidLocations := make(map[string][]string)
	for _, rel := range includedRels {
		abs := filepath.Join(dataDir, rel)
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if m := journalFIDRe.FindStringSubmatch(line); m != nil {
				loc := fmt.Sprintf("%s:%d", rel, i+1)
				fidLocations[m[1]] = append(fidLocations[m[1]], loc)
			}
		}
	}
	duplicateFIDs := make(map[string][]string)
	for fid, locs := range fidLocations {
		if len(locs) > 1 {
			duplicateFIDs[fid] = locs
		}
	}

	// Find .journal files on disk that aren't in the included set
	var unincludedFiles []string
	_ = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(path) == ".journal" && path != mainPath {
			if !includedSet[path] {
				rel, relErr := filepath.Rel(dataDir, path)
				if relErr != nil {
					rel = path
				}
				unincludedFiles = append(unincludedFiles, rel)
			}
		}
		return nil
	})

	ok := len(missingIncludes) == 0 && len(duplicateFIDs) == 0 && len(unincludedFiles) == 0

	result := auditResult{
		MissingIncludes: missingIncludes,
		DuplicateFIDs:   duplicateFIDs,
		UnincludedFiles: unincludedFiles,
		OK:              ok,
	}
	// Ensure nil slices serialize as [] not null
	if result.MissingIncludes == nil {
		result.MissingIncludes = []string{}
	}
	if result.UnincludedFiles == nil {
		result.UnincludedFiles = []string{}
	}

	if encErr := printJSON(result); encErr != nil {
		return encErr
	}
	if !ok {
		return fmt.Errorf("journal audit found issues")
	}
	return nil
}

// stringSliceFlag is a repeatable string flag (implements flag.Value).
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// postingSplitRe splits a posting string on 2+ consecutive spaces.
var postingSplitRe = regexp.MustCompile(`\s{2,}`)

// parsePostingArg parses "account  amount" or just "account" into PostingInput.
func parsePostingArg(s string) journal.PostingInput {
	parts := postingSplitRe.Split(strings.TrimSpace(s), 2)
	if len(parts) == 2 {
		return journal.PostingInput{
			Account: strings.TrimSpace(parts[0]),
			Amount:  strings.TrimSpace(parts[1]),
		}
	}
	return journal.PostingInput{Account: strings.TrimSpace(s)}
}

func runJournalDelete(args []string) error {
	fset := flag.NewFlagSet("journal delete", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal delete <data-dir> <fid>")
	}
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() < 2 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> and/or <fid> argument")
	}
	dataDir := fset.Arg(0)
	fid := fset.Arg(1)
	mainJournal := filepath.Join(dataDir, "main.journal")

	client, err := hledgerClient(mainJournal)
	if err != nil {
		return err
	}
	lock := txlock.New(dataDir, client)
	if err := lock.Do(context.Background(), func() error {
		return journal.DeleteTransaction(context.Background(), client, dataDir, fid)
	}); err != nil {
		return err
	}
	fmt.Printf("deleted transaction %s\n", fid)
	return nil
}

func runJournalAdd(args []string) error {
	fset := flag.NewFlagSet("journal add", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal add <data-dir> --description <text> --posting \"account  amount\" [--posting ...] [--date YYYY-MM-DD] [--comment <text>]")
	}
	dateStr := fset.String("date", "", "transaction date (YYYY-MM-DD, default: today)")
	description := fset.String("description", "", "transaction description (required)")
	comment := fset.String("comment", "", "optional transaction-level comment")
	var postings stringSliceFlag
	fset.Var(&postings, "posting", "posting in \"account  [amount]\" format (repeatable, min 2)")

	// Extract <data-dir> (first positional arg) before flag parsing, since Go's
	// flag package stops at the first non-flag argument.
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := args[0]
	if err := fset.Parse(args[1:]); err != nil {
		return err
	}

	if *description == "" {
		fset.Usage()
		return fmt.Errorf("--description is required")
	}
	if len(postings) < 2 {
		fset.Usage()
		return fmt.Errorf("at least 2 --posting entries are required")
	}

	var date time.Time
	if *dateStr == "" {
		date = time.Now().UTC().Truncate(24 * time.Hour)
	} else {
		var err error
		date, err = time.Parse("2006-01-02", *dateStr)
		if err != nil {
			return fmt.Errorf("invalid --date %q: expected YYYY-MM-DD", *dateStr)
		}
	}

	var postingInputs []journal.PostingInput
	for _, p := range postings {
		postingInputs = append(postingInputs, parsePostingArg(p))
	}

	tx := journal.TransactionInput{
		Date:        date,
		Description: *description,
		Comment:     *comment,
		Postings:    postingInputs,
	}

	mainJournal := filepath.Join(dataDir, "main.journal")
	client, err := hledgerClient(mainJournal)
	if err != nil {
		return err
	}
	lock := txlock.New(dataDir, client)

	var fid string
	if err := lock.Do(context.Background(), func() error {
		var addErr error
		fid, addErr = journal.AppendTransaction(context.Background(), client, dataDir, tx)
		return addErr
	}); err != nil {
		return err
	}
	fmt.Printf("added transaction with code: %s\n", fid)
	return nil
}

func runJournalImport(args []string) error {
	fset := flag.NewFlagSet("journal import", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl journal import <data-dir> <csv> --profile <name> [--yes]")
	}
	profileName := fset.String("profile", "", "bank profile name from config.toml (required)")
	yes := fset.Bool("yes", false, "skip confirmation prompt")

	// Extract <data-dir> and <csv> (positional args) before flag parsing, since Go's
	// flag package stops at the first non-flag argument.
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> and/or <csv> argument")
	}
	if len(args) < 2 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> and/or <csv> argument")
	}
	dataDir := args[0]
	csvFile := args[1]
	if err := fset.Parse(args[2:]); err != nil {
		return err
	}

	if *profileName == "" {
		fset.Usage()
		return fmt.Errorf("--profile is required")
	}

	// Load config and find bank profile.
	cfg, err := config.Load(filepath.Join(dataDir, "config.toml"))
	if err != nil {
		return fmt.Errorf("import: load config: %w", err)
	}
	var profile *config.BankProfile
	for i := range cfg.BankProfiles {
		if cfg.BankProfiles[i].Name == *profileName {
			profile = &cfg.BankProfiles[i]
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("import: bank profile %q not found in config.toml", *profileName)
	}
	rulesFile := filepath.Join(dataDir, profile.RulesFile)

	mainJournal := filepath.Join(dataDir, "main.journal")
	client, err := hledgerClient(mainJournal)
	if err != nil {
		return err
	}
	ctx := context.Background()

	// Parse CSV candidates.
	candidates, err := client.PrintCSV(ctx, csvFile, rulesFile)
	if err != nil {
		return fmt.Errorf("import: parse csv: %w", err)
	}

	// Fetch existing transactions for duplicate detection.
	existing, err := client.Transactions(ctx)
	if err != nil {
		return fmt.Errorf("import: fetch existing transactions: %w", err)
	}
	existingFPs := make(map[string]bool, len(existing))
	for _, t := range existing {
		existingFPs[txnFingerprint(t)] = true
	}

	// Classify candidates.
	type candidateEntry struct {
		txn hledger.Transaction
		dup bool
	}
	entries := make([]candidateEntry, len(candidates))
	for i, c := range candidates {
		entries[i] = candidateEntry{txn: c, dup: existingFPs[txnFingerprint(c)]}
	}

	// Print preview.
	for _, e := range entries {
		status := "[NEW]"
		if e.dup {
			status = "[DUP]"
		}
		fmt.Printf("%s  %s  %s\n", status, e.txn.Date, e.txn.Description)
		for _, p := range e.txn.Postings {
			amtStr := ""
			if len(p.Amounts) > 0 {
				a := p.Amounts[0]
				amtStr = fmt.Sprintf("  %s%.2f", a.Commodity, a.Quantity.FloatingPoint)
			}
			fmt.Printf("      %s%s\n", p.Account, amtStr)
		}
	}

	// Count new.
	var newCount, dupCount int
	for _, e := range entries {
		if e.dup {
			dupCount++
		} else {
			newCount++
		}
	}
	fmt.Printf("\n%d new transaction(s), %d duplicate(s)\n", newCount, dupCount)

	if newCount == 0 {
		fmt.Println("nothing to import")
		return nil
	}

	// Confirm unless --yes.
	if !*yes {
		fmt.Printf("Import %d transaction(s)? [y/N]: ", newCount)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "y" && answer != "Y" {
			fmt.Println("aborted")
			return nil
		}
	}

	// Write new transactions through txlock.
	lock := txlock.New(dataDir, client)
	imported := 0
	for _, e := range entries {
		if e.dup {
			continue
		}
		txInput, convErr := hledgerTxnToInput(e.txn)
		if convErr != nil {
			return fmt.Errorf("import: convert transaction: %w", convErr)
		}
		if err := lock.Do(ctx, func() error {
			_, writeErr := journal.AppendTransaction(ctx, client, dataDir, txInput)
			return writeErr
		}); err != nil {
			return fmt.Errorf("import: write transaction: %w", err)
		}
		imported++
	}
	fmt.Printf("imported %d transaction(s)\n", imported)
	return nil
}

// txnFingerprint returns a deduplication fingerprint for a transaction:
// date | description | sorted(account:amount) for each posting.
func txnFingerprint(t hledger.Transaction) string {
	parts := []string{t.Date, t.Description}
	var postings []string
	for _, p := range t.Postings {
		amtStr := ""
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			amtStr = fmt.Sprintf("%s%.6f", a.Commodity, a.Quantity.FloatingPoint)
		}
		postings = append(postings, p.Account+":"+amtStr)
	}
	sort.Strings(postings)
	parts = append(parts, postings...)
	return strings.Join(parts, "|")
}

// hledgerTxnToInput converts a parsed hledger.Transaction to a journal.TransactionInput
// suitable for AppendTransaction. All posting amounts are preserved explicitly.
func hledgerTxnToInput(t hledger.Transaction) (journal.TransactionInput, error) {
	date, err := time.Parse("2006-01-02", t.Date)
	if err != nil {
		return journal.TransactionInput{}, fmt.Errorf("parse date %q: %w", t.Date, err)
	}

	var postings []journal.PostingInput
	for _, p := range t.Postings {
		var amtStr string
		if len(p.Amounts) > 0 {
			a := p.Amounts[0]
			val := float64(a.Quantity.DecimalMantissa) / math.Pow10(a.Quantity.DecimalPlaces)
			amtStr = fmt.Sprintf("%s%.2f", a.Commodity, val)
		}
		postings = append(postings, journal.PostingInput{
			Account: p.Account,
			Amount:  amtStr,
			Comment: strings.TrimSpace(p.Comment),
		})
	}

	return journal.TransactionInput{
		Date:        date,
		Description: t.Description,
		Comment:     strings.TrimSpace(t.Comment),
		Postings:    postings,
	}, nil
}
