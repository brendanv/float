// floatctl is the admin and debug CLI for float. It operates directly on the data
// directory and internal packages, bypassing the gRPC API.
//
// Usage:
//
//	floatctl <group> <subcommand> [flags] [args...]
//	floatctl help
//	floatctl <group> help
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	group := os.Args[1]
	if group == "help" || group == "--help" || group == "-h" {
		printHelp()
		os.Exit(0)
	}

	if len(os.Args) < 3 {
		printGroupHelp(group)
		os.Exit(1)
	}

	name := os.Args[2]
	if name == "help" || name == "--help" || name == "-h" {
		printGroupHelp(group)
		os.Exit(0)
	}

	rest := os.Args[3:]
	if err := dispatch(group, name, rest); err != nil {
		fmt.Fprintf(os.Stderr, "floatctl: %v\n", err)
		os.Exit(1)
	}
}
