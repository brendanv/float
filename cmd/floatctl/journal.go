package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
)

func init() {
	register(
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

	rows, err := c.Register(context.Background(), "tag:fid="+fid)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no transaction found with fid %q", fid)
	}
	return printJSON(rows)
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
var journalFIDRe = regexp.MustCompile(`fid:([0-9a-f]{8})`)

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
