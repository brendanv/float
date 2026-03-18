// hledger-internal-test is a debug CLI for exercising the internal/hledger
// package against a real journal file and printing parsed output as JSON.
//
// Usage:
//
//	hledger-internal-test balance  <journal> [--depth N] [query...]
//	hledger-internal-test accounts <journal> [--tree]
//	hledger-internal-test register <journal> [query...]
//	hledger-internal-test print-csv <csv> <rules>
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/brendanv/float/internal/hledger"
)

func bgCtx() context.Context { return context.Background() }

const usage = `hledger-internal-test — debug CLI for the internal/hledger package

Subcommands:
  balance  [--depth N] <journal> [query...]   run hledger bal and print parsed BalanceReport
  accounts [--tree]   <journal>               run hledger accounts and print parsed AccountNodes
  register            <journal> [query...]    run hledger reg and print parsed RegisterRows
  print-csv           <csv> <rules>           run hledger print on a CSV file and print parsed Transactions

Note: flags must precede positional arguments (standard Go flag behavior).
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	sub := flag.Arg(0)
	rest := flag.Args()[1:]

	switch sub {
	case "balance":
		runBalance(rest)
	case "accounts":
		runAccounts(rest)
	case "register":
		runRegister(rest)
	case "print-csv":
		runPrintCSV(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", sub)
		flag.Usage()
		os.Exit(1)
	}
}

func mustClient(journal string) *hledger.Client {
	c, err := hledger.New("hledger", journal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return c
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
		os.Exit(1)
	}
}

func runBalance(args []string) {
	fs := flag.NewFlagSet("balance", flag.ExitOnError)
	depth := fs.Int("depth", 0, "limit account depth (0 = no limit)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: hledger-internal-test balance [--depth N] <journal> [query...]")
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}
	journal := fs.Arg(0)
	query := fs.Args()[1:]

	c := mustClient(journal)
	report, err := c.Balances(bgCtx(), *depth, query...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(report)
}

func runAccounts(args []string) {
	fs := flag.NewFlagSet("accounts", flag.ExitOnError)
	tree := fs.Bool("tree", false, "return account tree instead of flat list")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: hledger-internal-test accounts [--tree] <journal>")
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}
	journal := fs.Arg(0)

	c := mustClient(journal)
	nodes, err := c.Accounts(bgCtx(), *tree)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(nodes)
}

func runRegister(args []string) {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: hledger-internal-test register <journal> [query...]")
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}
	journal := fs.Arg(0)
	query := fs.Args()[1:]

	c := mustClient(journal)
	rows, err := c.Register(bgCtx(), query...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(rows)
}

func runPrintCSV(args []string) {
	fs := flag.NewFlagSet("print-csv", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: hledger-internal-test print-csv <csv> <rules>")
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if fs.NArg() < 2 {
		fs.Usage()
		os.Exit(1)
	}
	csvFile := fs.Arg(0)
	rulesFile := fs.Arg(1)

	// print-csv doesn't need a real journal — use /dev/null.
	c := mustClient("/dev/null")
	txns, err := c.PrintCSV(bgCtx(), csvFile, rulesFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printJSON(txns)
}
