package main

import (
	"errors"
	"testing"
)

func TestDispatch_Errors(t *testing.T) {
	tests := []struct {
		name  string
		group string
		cmd   string
	}{
		{"unknown group", "no-such-group", "cmd"},
		{"unknown subcommand", "hledger", "no-such-cmd"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := dispatch(tc.group, tc.cmd, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	sentinel := errors.New("sentinel")
	tests := []struct {
		name    string
		args    []string
		runErr  error
		wantErr error
	}{
		{name: "calls run", args: nil},
		{name: "passes args", args: []string{"a", "b"}},
		{name: "propagates error", runErr: sentinel, wantErr: sentinel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := registry
			t.Cleanup(func() { registry = orig })

			var gotArgs []string
			register(&Command{
				Group: "testgrp", Name: "testcmd", Synopsis: "test",
				Run: func(args []string) error { gotArgs = args; return tc.runErr },
			})

			err := dispatch("testgrp", "testcmd", tc.args)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error: got %v, want %v", err, tc.wantErr)
			}
			if len(gotArgs) != len(tc.args) {
				t.Errorf("args: got %v, want %v", gotArgs, tc.args)
			}
			for i, a := range tc.args {
				if gotArgs[i] != a {
					t.Errorf("args[%d]: got %q, want %q", i, gotArgs[i], a)
				}
			}
		})
	}
}
