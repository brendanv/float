package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/stripeconn"
	"github.com/brendanv/float/internal/txlock"
)

func init() {
	register(
		&Command{
			Group:    "stripe",
			Name:     "list",
			Synopsis: "List linked Stripe Financial Connections accounts and their hledger mapping",
			Run:      runStripeList,
		},
		&Command{
			Group:    "stripe",
			Name:     "sync",
			Synopsis: "Pull posted transactions from Stripe and import into the journal",
			Run:      runStripeSync,
		},
		&Command{
			Group:    "stripe",
			Name:     "set-key",
			Synopsis: "Write a Stripe API key to config.toml (use empty string to clear)",
			Run:      runStripeSetKey,
		},
	)
}

func runStripeList(args []string) error {
	fset := flag.NewFlagSet("stripe list", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl stripe list <data-dir>")
	}
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := args[0]
	if err := fset.Parse(args[1:]); err != nil {
		return err
	}

	store, err := stripeconn.Load(dataDir)
	if err != nil {
		return err
	}
	if len(store.Connections) == 0 {
		fmt.Println("no Stripe connections linked")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "ID\tDISPLAY NAME\tHLEDGER ACCOUNT\tLAST SYNCED\tIMPORTED"); err != nil {
		return err
	}
	for _, c := range store.Connections {
		last := "never"
		if !c.LastSyncedAt.IsZero() {
			last = c.LastSyncedAt.Format("2006-01-02 15:04")
		}
		mapping := c.HledgerAccount
		if mapping == "" {
			mapping = "(unmapped)"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", c.ID, c.DisplayName, mapping, last, len(c.ImportedIDs)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func runStripeSync(args []string) error {
	fset := flag.NewFlagSet("stripe sync", flag.ExitOnError)
	connID := fset.String("connection-id", "", "sync only this connection id (default: all mapped connections)")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl stripe sync <data-dir> [--connection-id ID]")
	}
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := args[0]
	if err := fset.Parse(args[1:]); err != nil {
		return err
	}
	ctx := context.Background()

	cfg, err := config.Load(filepath.Join(dataDir, "config.toml"))
	if err != nil {
		return err
	}
	if cfg.Stripe.APIKey == "" {
		return fmt.Errorf("stripe.api_key is not configured in %s/config.toml", dataDir)
	}

	store, err := stripeconn.Load(dataDir)
	if err != nil {
		return err
	}

	client, err := hledger.New("hledger", filepath.Join(dataDir, "main.journal"))
	if err != nil {
		return err
	}
	lock := txlock.New(dataDir, client)
	if snap, err := gitsnap.New(dataDir); err == nil {
		lock.SetSnap(snap)
	}

	api, err := stripeconn.NewLiveStripe(cfg.Stripe.APIKey)
	if err != nil {
		return err
	}
	rulesList, err := rules.Load(dataDir)
	if err != nil {
		return err
	}

	targets := store.Connections
	if *connID != "" {
		found := store.Find(*connID)
		if found == nil {
			return fmt.Errorf("connection %s not found", *connID)
		}
		targets = []stripeconn.Connection{*found}
	}

	totalImported, totalSkipped := 0, 0
	for _, c := range targets {
		if c.HledgerAccount == "" {
			fmt.Printf("skipping %s (%s): no hledger account mapping\n", c.ID, c.DisplayName)
			continue
		}
		res, err := stripeconn.Sync(ctx, client, lock, dataDir, c.ID, rulesList, api)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync %s (%s): %v\n", c.ID, c.DisplayName, err)
			continue
		}
		fmt.Printf("%s (%s): imported=%d skipped=%d\n", c.ID, c.DisplayName, res.Imported, res.Skipped)
		totalImported += res.Imported
		totalSkipped += res.Skipped
	}
	fmt.Printf("done: imported=%d skipped=%d\n", totalImported, totalSkipped)
	return nil
}

func runStripeSetKey(args []string) error {
	fset := flag.NewFlagSet("stripe set-key", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl stripe set-key <data-dir> <api-key>")
		fmt.Fprintln(os.Stderr, "  Use an empty <api-key> to clear the configured key.")
	}
	if len(args) < 2 {
		fset.Usage()
		return fmt.Errorf("missing arguments")
	}
	dataDir := args[0]
	key := args[1]

	cfgPath := filepath.Join(dataDir, "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	cfg.Stripe.APIKey = key
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	if key == "" {
		fmt.Println("stripe api key cleared")
	} else {
		fmt.Println("stripe api key updated")
	}
	return nil
}
