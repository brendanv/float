package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
