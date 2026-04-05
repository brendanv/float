package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/txlock"
)

func init() {
	register(&Command{
		Group:    "rules",
		Name:     "list",
		Synopsis: "List categorization rules as JSON",
		Run:      runRulesList,
	})
	register(&Command{
		Group:    "rules",
		Name:     "add",
		Synopsis: "Add a new categorization rule",
		Run:      runRulesAdd,
	})
	register(&Command{
		Group:    "rules",
		Name:     "delete",
		Synopsis: "Delete a categorization rule by ID",
		Run:      runRulesDelete,
	})
	register(&Command{
		Group:    "rules",
		Name:     "apply",
		Synopsis: "Preview and apply categorization rules retroactively",
		Run:      runRulesApply,
	})
}

func runRulesList(args []string) error {
	fset := flag.NewFlagSet("rules list", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl rules list <data-dir>")
	}
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := args[0]
	if err := fset.Parse(args[1:]); err != nil {
		return err
	}

	rulesList, err := rules.Load(dataDir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rulesList)
}

func runRulesAdd(args []string) error {
	fset := flag.NewFlagSet("rules add", flag.ExitOnError)
	pattern := fset.String("pattern", "", "regex pattern to match against description (required)")
	payee := fset.String("payee", "", "set payee (empty = no change)")
	account := fset.String("account", "", "set category account (empty = no change)")
	priority := fset.Int("priority", 0, "rule priority (lower = higher priority)")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl rules add <data-dir> --pattern REGEX [--payee PAYEE] [--account ACCOUNT] [--priority N]")
		fset.PrintDefaults()
	}
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := args[0]
	if err := fset.Parse(args[1:]); err != nil {
		return err
	}
	if *pattern == "" {
		return fmt.Errorf("--pattern is required")
	}

	ctx := context.Background()
	client, err := newHledgerClient(dataDir)
	if err != nil {
		return err
	}
	lock := txlock.New(dataDir, client)

	var newRule rules.Rule
	if err := lock.Do(ctx, "add rule", func() error {
		rulesList, loadErr := rules.Load(dataDir)
		if loadErr != nil {
			return loadErr
		}
		newRule = rules.Rule{
			ID:       journal.MintFID(),
			Pattern:  *pattern,
			Payee:    *payee,
			Account:  *account,
			Priority: *priority,
		}
		rulesList = append(rulesList, newRule)
		return rules.Save(dataDir, rulesList)
	}); err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(newRule)
}

func runRulesDelete(args []string) error {
	fset := flag.NewFlagSet("rules delete", flag.ExitOnError)
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl rules delete <data-dir> <rule-id>")
	}
	if len(args) < 2 {
		fset.Usage()
		return fmt.Errorf("missing arguments: need <data-dir> and <rule-id>")
	}
	dataDir := args[0]
	ruleID := args[1]
	if err := fset.Parse(args[2:]); err != nil {
		return err
	}

	ctx := context.Background()
	client, err := newHledgerClient(dataDir)
	if err != nil {
		return err
	}
	lock := txlock.New(dataDir, client)

	return lock.Do(ctx, "delete rule", func() error {
		rulesList, loadErr := rules.Load(dataDir)
		if loadErr != nil {
			return loadErr
		}
		filtered := rulesList[:0]
		found := false
		for _, r := range rulesList {
			if r.ID == ruleID {
				found = true
				continue
			}
			filtered = append(filtered, r)
		}
		if !found {
			return fmt.Errorf("rule %q not found", ruleID)
		}
		if err := rules.Save(dataDir, filtered); err != nil {
			return err
		}
		fmt.Printf("deleted rule %s\n", ruleID)
		return nil
	})
}

func runRulesApply(args []string) error {
	fset := flag.NewFlagSet("rules apply", flag.ExitOnError)
	yes := fset.Bool("yes", false, "apply without confirmation prompt")
	ruleID := fset.String("rule-id", "", "apply only this rule ID (empty = all rules)")
	fset.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl rules apply <data-dir> [--rule-id ID] [--yes] [query...]")
		fset.PrintDefaults()
	}
	if len(args) < 1 {
		fset.Usage()
		return fmt.Errorf("missing <data-dir> argument")
	}
	dataDir := args[0]
	if err := fset.Parse(args[1:]); err != nil {
		return err
	}
	query := fset.Args()

	rulesList, err := rules.Load(dataDir)
	if err != nil {
		return err
	}
	if *ruleID != "" {
		filtered := rulesList[:0]
		for _, r := range rulesList {
			if r.ID == *ruleID {
				filtered = append(filtered, r)
			}
		}
		rulesList = filtered
		if len(rulesList) == 0 {
			return fmt.Errorf("rule %q not found", *ruleID)
		}
	}
	if len(rulesList) == 0 {
		fmt.Println("no rules defined")
		return nil
	}

	client, err := newHledgerClient(dataDir)
	if err != nil {
		return err
	}

	ctx := context.Background()
	txns, err := client.Transactions(ctx, query...)
	if err != nil {
		return fmt.Errorf("fetch transactions: %w", err)
	}

	matches := rules.Preview(rulesList, txns)
	if len(matches) == 0 {
		fmt.Println("no transactions match any rules")
		return nil
	}

	// Print preview.
	fmt.Printf("%d transaction(s) would be updated:\n\n", len(matches))
	for _, m := range matches {
		fmt.Printf("  [%s] %s  %s\n", m.Transaction.FID, m.Transaction.Date, m.Transaction.Description)
		fmt.Printf("    rule: %s (pattern: %s)\n", m.Rule.ID, m.Rule.Pattern)
		if m.Changes.NewPayee != nil {
			fmt.Printf("    payee: → %q\n", *m.Changes.NewPayee)
		}
		if m.Changes.NewAccount != nil {
			fmt.Printf("    account: → %q\n", *m.Changes.NewAccount)
		}
		for k, v := range m.Changes.AddTags {
			fmt.Printf("    tag: %s=%s\n", k, v)
		}
	}
	fmt.Println()

	if !*yes {
		fmt.Printf("Apply changes to %d transaction(s)? [y/N]: ", len(matches))
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("aborted")
			return nil
		}
	}

	lock := txlock.New(dataDir, client)
	var applied int
	if err := lock.Do(ctx, "apply rules", func() error {
		var applyErr error
		applied, applyErr = rules.Apply(ctx, client, dataDir, matches)
		return applyErr
	}); err != nil {
		return fmt.Errorf("apply rules: %w", err)
	}

	fmt.Printf("applied %s to %d transaction(s)\n", pluralRule(applied), applied)
	return nil
}

func pluralRule(n int) string {
	if n == 1 {
		return "1 rule"
	}
	return fmt.Sprintf("%d rules", n)
}

// newHledgerClient creates a hledger client for the given data directory.
func newHledgerClient(dataDir string) (*hledger.Client, error) {
	return hledger.New("hledger", dataDir+"/main.journal")
}
