package hledger_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/hledger"
)

// TestQueryTermValidation guards against hledger flag injection: hledger
// accepts flags anywhere in argv, so a query token like --output-file=FILE
// would overwrite files outside the txlock write protocol.
func TestQueryTermValidation(t *testing.T) {
	c := mustClient(t, "simple.journal")
	ctx := t.Context()

	calls := []struct {
		name string
		call func(query ...string) error
	}{
		{"Transactions", func(q ...string) error { _, err := c.Transactions(ctx, q...); return err }},
		{"Balances", func(q ...string) error { _, err := c.Balances(ctx, 0, q...); return err }},
		{"BalancesValued", func(q ...string) error { _, err := c.BalancesValued(ctx, "now,USD", 0, q...); return err }},
		{"BalancesCost", func(q ...string) error { _, err := c.BalancesCost(ctx, 0, q...); return err }},
		{"Register", func(q ...string) error { _, err := c.Register(ctx, q...); return err }},
		{"Aregister", func(q ...string) error { _, err := c.Aregister(ctx, "assets:checking", q...); return err }},
	}
	unsafeQueries := [][]string{
		{"--output-file=/tmp/clobbered.journal"},
		{"-f", "/etc/hostname"},
		{"expenses", "--rules-file", "x"},
	}

	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call("expenses"); err != nil {
				t.Errorf("plain query term rejected: %v", err)
			}
			for _, q := range unsafeQueries {
				err := tc.call(q...)
				if !errors.Is(err, hledger.ErrUnsafeQuery) {
					t.Errorf("query %v: error = %v, want ErrUnsafeQuery", q, err)
				}
			}
		})
	}

	t.Run("Aregister_account_positional", func(t *testing.T) {
		_, err := c.Aregister(ctx, "--output-file=/tmp/x")
		if !errors.Is(err, hledger.ErrUnsafeQuery) {
			t.Errorf("error = %v, want ErrUnsafeQuery", err)
		}
	})
}

func TestRunQueryRestrictions(t *testing.T) {
	c := mustClient(t, "simple.journal")
	ctx := t.Context()

	t.Run("allows_readonly_commands", func(t *testing.T) {
		for _, args := range []string{"bal", "print expenses", "stats", "reg --depth 2"} {
			stdout, _, _, err := c.RunQuery(ctx, args)
			if err != nil {
				t.Errorf("RunQuery(%q): %v", args, err)
			}
			if len(stdout) == 0 {
				t.Errorf("RunQuery(%q): empty stdout", args)
			}
		}
	})

	t.Run("rejects_writing_commands_and_redirects", func(t *testing.T) {
		for _, args := range []string{
			"import statement.csv",
			"rewrite expenses --add-posting 'x  $1'",
			"add",
			"print --output-file /tmp/x.journal",
			"print -o /tmp/x.journal",
			"bal -f /etc/hostname",
			"print --rules-file evil.rules",
			"",
		} {
			_, _, _, err := c.RunQuery(ctx, args)
			if !errors.Is(err, hledger.ErrUnsafeQuery) {
				t.Errorf("RunQuery(%q): error = %v, want ErrUnsafeQuery", args, err)
			}
		}
	})

	t.Run("error_names_offending_argument", func(t *testing.T) {
		_, _, _, err := c.RunQuery(ctx, "import statement.csv")
		if err == nil || !strings.Contains(err.Error(), "import") {
			t.Errorf("error should name the rejected command, got: %v", err)
		}
	})
}
