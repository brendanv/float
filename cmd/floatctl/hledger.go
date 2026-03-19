package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/brendanv/float/internal/hledger"
)

func init() {
	register(
		&Command{
			Group:    "hledger",
			Name:     "balance",
			Synopsis: "Run hledger bal -O json and print parsed BalanceReport as JSON",
			Run:      runHledgerBalance,
		},
		&Command{
			Group:    "hledger",
			Name:     "accounts",
			Synopsis: "Run hledger accounts and print parsed AccountNode tree as JSON",
			Run:      runHledgerAccounts,
		},
		&Command{
			Group:    "hledger",
			Name:     "register",
			Synopsis: "Run hledger reg -O json and print parsed RegisterRows as JSON",
			Run:      runHledgerRegister,
		},
		&Command{
			Group:    "hledger",
			Name:     "print-csv",
			Synopsis: "Run hledger print on a CSV+rules file and print parsed Transactions as JSON",
			Run:      runHledgerPrintCSV,
		},
		&Command{
			Group:    "hledger",
			Name:     "version",
			Synopsis: "Print the hledger binary version string",
			Run:      runHledgerVersion,
		},
		&Command{
			Group:    "hledger",
			Name:     "check",
			Synopsis: "Run hledger check on a journal; exit 0 if valid, 1 with error message",
			Run:      runHledgerCheck,
		},
		&Command{
			Group:    "hledger",
			Name:     "raw",
			Synopsis: "Run any hledger subcommand with arbitrary args; print raw stdout (escape hatch for debugging)",
			Run:      runHledgerRaw,
		},
	)
}

func hledgerClient(journal string) (*hledger.Client, error) {
	return hledger.New("hledger", journal)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func runHledgerBalance(args []string) error {
	fs := flag.NewFlagSet("hledger balance", flag.ExitOnError)
	depth := fs.Int("depth", 0, "limit account depth (0 = no limit)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger balance [--depth N] <journal> [query...]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <journal> argument")
	}
	journal := fs.Arg(0)
	query := fs.Args()[1:]

	c, err := hledgerClient(journal)
	if err != nil {
		return err
	}
	report, err := c.Balances(context.Background(), *depth, query...)
	if err != nil {
		return err
	}
	return printJSON(report)
}

func runHledgerAccounts(args []string) error {
	fs := flag.NewFlagSet("hledger accounts", flag.ExitOnError)
	tree := fs.Bool("tree", false, "return account tree instead of flat list")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger accounts [--tree] <journal>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <journal> argument")
	}
	journal := fs.Arg(0)

	c, err := hledgerClient(journal)
	if err != nil {
		return err
	}
	nodes, err := c.Accounts(context.Background(), *tree)
	if err != nil {
		return err
	}
	return printJSON(nodes)
}

func runHledgerRegister(args []string) error {
	fs := flag.NewFlagSet("hledger register", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger register <journal> [query...]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <journal> argument")
	}
	journal := fs.Arg(0)
	query := fs.Args()[1:]

	c, err := hledgerClient(journal)
	if err != nil {
		return err
	}
	rows, err := c.Register(context.Background(), query...)
	if err != nil {
		return err
	}
	return printJSON(rows)
}

func runHledgerPrintCSV(args []string) error {
	fs := flag.NewFlagSet("hledger print-csv", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger print-csv <csv> <rules>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return fmt.Errorf("missing <csv> and/or <rules> arguments")
	}
	csvFile := fs.Arg(0)
	rulesFile := fs.Arg(1)

	// print-csv doesn't need a real journal — use /dev/null.
	c, err := hledgerClient("/dev/null")
	if err != nil {
		return err
	}
	txns, err := c.PrintCSV(context.Background(), csvFile, rulesFile)
	if err != nil {
		return err
	}
	return printJSON(txns)
}

func runHledgerVersion(args []string) error {
	fs := flag.NewFlagSet("hledger version", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger version")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Version check doesn't need a real journal.
	c, err := hledgerClient("/dev/null")
	if err != nil {
		return err
	}
	version, err := c.Version(context.Background())
	if err != nil {
		return err
	}
	fmt.Println(version)
	return nil
}

func runHledgerCheck(args []string) error {
	fs := flag.NewFlagSet("hledger check", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger check <journal>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <journal> argument")
	}
	journal := fs.Arg(0)

	c, err := hledgerClient(journal)
	if err != nil {
		return err
	}
	if err := c.Check(context.Background()); err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func runHledgerRaw(args []string) error {
	fs := flag.NewFlagSet("hledger raw", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl hledger raw <journal> <subcmd> [args...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Runs hledger with arbitrary args and prints raw stdout.")
		fmt.Fprintln(os.Stderr, "The exact command is printed to stderr as '# command: ...'.")
		fmt.Fprintln(os.Stderr, "Useful for debugging parse failures and unexpected hledger output.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		fs.Usage()
		return fmt.Errorf("missing <journal> and/or <subcmd> argument")
	}
	journal := fs.Arg(0)
	subcmd := fs.Arg(1)
	rest := fs.Args()[2:]

	c, err := hledgerClient(journal)
	if err != nil {
		return err
	}

	hledgerArgs := append([]string{subcmd, "-f", journal}, rest...)
	stdout, stderr, cmdLine, err := c.RunRaw(context.Background(), hledgerArgs...)

	fmt.Fprintf(os.Stderr, "# command: %s\n", cmdLine)
	if len(stderr) > 0 {
		fmt.Fprintf(os.Stderr, "# stderr:\n%s\n", stderr)
	}
	if len(stdout) > 0 {
		_, _ = os.Stdout.Write(stdout)
	}
	return err
}
