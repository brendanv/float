package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brendanv/float/internal/config"
)

func init() {
	register(
		&Command{
			Group:    "config",
			Name:     "show",
			Synopsis: "Print parsed config.toml as JSON",
			Run:      runConfigShow,
		},
		&Command{
			Group:    "config",
			Name:     "validate",
			Synopsis: "Validate config.toml; exit 0 if valid, 1 with errors",
			Run:      runConfigValidate,
		},
	)
}

func runConfigShow(args []string) error {
	fs := flag.NewFlagSet("config show", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl config show <config.toml>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <config.toml> argument")
	}

	cfg, err := config.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	return printJSON(cfg)
}

func runConfigValidate(args []string) error {
	fs := flag.NewFlagSet("config validate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: floatctl config validate <config.toml>")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("missing <config.toml> argument")
	}

	if _, err := config.Load(fs.Arg(0)); err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}
