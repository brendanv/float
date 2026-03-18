package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Command is a single floatctl subcommand belonging to a group.
type Command struct {
	Group    string
	Name     string
	Synopsis string
	Run      func(args []string) error
}

var registry []*Command

// register adds commands to the global registry. Call from init() in each group file.
func register(cmds ...*Command) {
	registry = append(registry, cmds...)
}

// dispatch finds the command matching group+name and calls its Run function.
// Returns an error if the group or command is not found.
func dispatch(group, name string, args []string) error {
	for _, cmd := range registry {
		if cmd.Group == group && cmd.Name == name {
			return cmd.Run(args)
		}
	}
	// Check if the group exists at all for a better error message.
	for _, cmd := range registry {
		if cmd.Group == group {
			fmt.Fprintf(os.Stderr, "floatctl: unknown subcommand %q for group %q\n\n", name, group)
			printGroupHelp(group)
			return fmt.Errorf("unknown subcommand %q", name)
		}
	}
	fmt.Fprintf(os.Stderr, "floatctl: unknown group %q\n\n", group)
	printHelp()
	return fmt.Errorf("unknown group %q", group)
}

// printHelp prints top-level usage listing all groups and their commands.
func printHelp() {
	fmt.Fprintln(os.Stderr, "floatctl — admin and debug CLI for float")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  floatctl <group> <subcommand> [flags] [args...]")
	fmt.Fprintln(os.Stderr, "  floatctl help")
	fmt.Fprintln(os.Stderr, "  floatctl <group> help")
	fmt.Fprintln(os.Stderr, "")

	// Collect groups in sorted order.
	groups := make(map[string][]*Command)
	var groupOrder []string
	for _, cmd := range registry {
		if _, ok := groups[cmd.Group]; !ok {
			groupOrder = append(groupOrder, cmd.Group)
		}
		groups[cmd.Group] = append(groups[cmd.Group], cmd)
	}
	sort.Strings(groupOrder)

	fmt.Fprintln(os.Stderr, "Groups:")
	for _, g := range groupOrder {
		cmds := groups[g]
		sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
		names := make([]string, len(cmds))
		for i, c := range cmds {
			names[i] = c.Name
		}
		fmt.Fprintf(os.Stderr, "  %-14s  %s\n", g, strings.Join(names, ", "))
	}
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprint(os.Stderr, "Run 'floatctl <group> help' to list subcommands for a group.\n")
}

// printGroupHelp prints all subcommands for a given group.
func printGroupHelp(group string) {
	var cmds []*Command
	for _, cmd := range registry {
		if cmd.Group == group {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		fmt.Fprintf(os.Stderr, "floatctl: no commands registered for group %q\n", group)
		return
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })

	fmt.Fprintf(os.Stderr, "floatctl %s — subcommands:\n\n", group)
	for _, cmd := range cmds {
		fmt.Fprintf(os.Stderr, "  %-16s  %s\n", cmd.Name, cmd.Synopsis)
	}
	fmt.Fprintln(os.Stderr, "")
}
