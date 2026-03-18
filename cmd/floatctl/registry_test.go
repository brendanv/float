package main

import (
	"errors"
	"testing"
)

func TestDispatch_UnknownGroup(t *testing.T) {
	err := dispatch("no-such-group", "cmd", nil)
	if err == nil {
		t.Fatal("expected error for unknown group")
	}
}

func TestDispatch_UnknownSubcommand(t *testing.T) {
	// "hledger" group is registered by hledger.go init().
	err := dispatch("hledger", "no-such-cmd", nil)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestDispatch_CallsRun(t *testing.T) {
	orig := registry
	t.Cleanup(func() { registry = orig })

	called := false
	register(&Command{
		Group: "testgrp", Name: "testcmd", Synopsis: "test",
		Run: func(args []string) error { called = true; return nil },
	})

	if err := dispatch("testgrp", "testcmd", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected Run to be called")
	}
}

func TestDispatch_PassesArgs(t *testing.T) {
	orig := registry
	t.Cleanup(func() { registry = orig })

	var got []string
	register(&Command{
		Group: "testgrp", Name: "testargs", Synopsis: "test",
		Run: func(args []string) error { got = args; return nil },
	})

	if err := dispatch("testgrp", "testargs", []string{"a", "b"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("expected [a b], got %v", got)
	}
}

func TestDispatch_PropagatesError(t *testing.T) {
	orig := registry
	t.Cleanup(func() { registry = orig })

	wantErr := errors.New("command failed")
	register(&Command{
		Group: "testgrp", Name: "testerr", Synopsis: "test",
		Run: func(args []string) error { return wantErr },
	})

	err := dispatch("testgrp", "testerr", nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wantErr, got %v", err)
	}
}
