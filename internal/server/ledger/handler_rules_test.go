package ledger_test

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/rules"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
	"github.com/brendanv/float/internal/txlock"
)

// mustRealHandlerWithSnap creates a handler with a real hledger client, txlock,
// and a gitsnap repo wired up so that each successful write produces a commit.
func mustRealHandlerWithSnap(t *testing.T, dir string) (*serverledger.Handler, *gitsnap.Repo) {
	t.Helper()
	c, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	snap, err := gitsnap.New(dir)
	if err != nil {
		t.Fatalf("gitsnap.New: %v", err)
	}
	lock := txlock.New(dir, c)
	lock.SetSnap(snap)
	return serverledger.NewHandler(c, lock, dir, "", nil, snap, nil, nil), snap
}

func TestAddRuleHandler(t *testing.T) {
	t.Run("empty_rules_list_returns_invalid_argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 60, NumTxns: 1})
		h := mustRealHandler(t, dir)
		_, err := h.AddRule(t.Context(), connect.NewRequest(&floatv1.AddRuleRequest{
			Rules: []*floatv1.RuleInput{},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("empty_pattern_returns_invalid_argument", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 61, NumTxns: 1})
		h := mustRealHandler(t, dir)
		_, err := h.AddRule(t.Context(), connect.NewRequest(&floatv1.AddRuleRequest{
			Rules: []*floatv1.RuleInput{
				{Pattern: "AMAZON"},
				{Pattern: ""},
			},
		}))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("single_rule_saved", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 62, NumTxns: 1})
		h := mustRealHandler(t, dir)
		resp, err := h.AddRule(t.Context(), connect.NewRequest(&floatv1.AddRuleRequest{
			Rules: []*floatv1.RuleInput{
				{Pattern: "AMAZON", Payee: "Amazon", Account: "expenses:shopping", Priority: 10, AutoReviewed: true},
			},
		}))
		if err != nil {
			t.Fatalf("AddRule: %v", err)
		}
		if len(resp.Msg.Rules) != 1 {
			t.Fatalf("got %d rules in response, want 1", len(resp.Msg.Rules))
		}
		got := resp.Msg.Rules[0]
		if got.Id == "" {
			t.Error("rule ID should not be empty")
		}
		if got.Pattern != "AMAZON" {
			t.Errorf("pattern = %q, want %q", got.Pattern, "AMAZON")
		}
		if got.Payee != "Amazon" {
			t.Errorf("payee = %q, want %q", got.Payee, "Amazon")
		}
		if got.Account != "expenses:shopping" {
			t.Errorf("account = %q, want %q", got.Account, "expenses:shopping")
		}
		if int(got.Priority) != 10 {
			t.Errorf("priority = %d, want 10", got.Priority)
		}
		if !got.AutoReviewed {
			t.Error("auto_reviewed should be true")
		}

		loaded, err := rules.Load(dir)
		if err != nil {
			t.Fatalf("rules.Load: %v", err)
		}
		if len(loaded) != 1 {
			t.Errorf("got %d rules on disk, want 1", len(loaded))
		}
		if loaded[0].ID != got.Id {
			t.Errorf("on-disk ID = %q, want %q", loaded[0].ID, got.Id)
		}
	})

	t.Run("multiple_rules_saved_in_single_snapshot", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 63, NumTxns: 1})
		h, snap := mustRealHandlerWithSnap(t, dir)

		initialSnaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List initial snapshots: %v", err)
		}
		initialCount := len(initialSnaps)

		resp, err := h.AddRule(t.Context(), connect.NewRequest(&floatv1.AddRuleRequest{
			Rules: []*floatv1.RuleInput{
				{Pattern: "AMAZON", Payee: "Amazon", Account: "expenses:shopping"},
				{Pattern: "STARBUCKS", Payee: "Starbucks", Account: "expenses:food:coffee"},
				{Pattern: "NETFLIX", Account: "expenses:subscriptions"},
			},
		}))
		if err != nil {
			t.Fatalf("AddRule: %v", err)
		}
		if len(resp.Msg.Rules) != 3 {
			t.Errorf("got %d rules in response, want 3", len(resp.Msg.Rules))
		}
		for i, r := range resp.Msg.Rules {
			if r.Id == "" {
				t.Errorf("rules[%d].Id should not be empty", i)
			}
		}

		// All three rules must land in exactly one new git snapshot.
		afterSnaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List snapshots after AddRule: %v", err)
		}
		if len(afterSnaps) != initialCount+1 {
			t.Errorf("snapshot count: got %d, want %d (one new commit)", len(afterSnaps), initialCount+1)
		}

		// All three rules must be persisted on disk.
		loaded, err := rules.Load(dir)
		if err != nil {
			t.Fatalf("rules.Load: %v", err)
		}
		if len(loaded) != 3 {
			t.Errorf("got %d rules on disk, want 3", len(loaded))
		}
	})

	t.Run("bulk_rule_snapshot_message_is_capped", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 65, NumTxns: 1})
		h, snap := mustRealHandlerWithSnap(t, dir)

		req := &floatv1.AddRuleRequest{
			Rules: []*floatv1.RuleInput{
				{Pattern: "THIS_IS_A_VERY_LONG_RULE_PATTERN_ALPHA_1234567890"},
				{Pattern: "THIS_IS_A_VERY_LONG_RULE_PATTERN_BETA_1234567890"},
				{Pattern: "THIS_IS_A_VERY_LONG_RULE_PATTERN_GAMMA_1234567890"},
				{Pattern: "THIS_IS_A_VERY_LONG_RULE_PATTERN_DELTA_1234567890"},
			},
		}
		if _, err := h.AddRule(t.Context(), connect.NewRequest(req)); err != nil {
			t.Fatalf("AddRule: %v", err)
		}

		snaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List snapshots: %v", err)
		}
		if len(snaps) == 0 {
			t.Fatal("expected at least one snapshot")
		}
		msg := snaps[0].Message
		if got, max := len(msg), 180; got > max {
			t.Fatalf("snapshot message too long: got %d, want <= %d. msg=%q", got, max, msg)
		}
		if !strings.Contains(msg, "+") || !strings.Contains(msg, "more") {
			t.Fatalf("expected truncated bulk message with '+N more', got %q", msg)
		}
	})

	t.Run("second_call_produces_second_snapshot", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 64, NumTxns: 1})
		h, snap := mustRealHandlerWithSnap(t, dir)

		initialSnaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List initial: %v", err)
		}
		initialCount := len(initialSnaps)

		for _, patterns := range [][]*floatv1.RuleInput{
			{{Pattern: "AMAZON", Account: "expenses:shopping"}},
			{{Pattern: "STARBUCKS", Account: "expenses:food"}, {Pattern: "NETFLIX", Account: "expenses:subscriptions"}},
		} {
			if _, err := h.AddRule(t.Context(), connect.NewRequest(&floatv1.AddRuleRequest{Rules: patterns})); err != nil {
				t.Fatalf("AddRule: %v", err)
			}
		}

		afterSnaps, err := snap.List(t.Context(), 20)
		if err != nil {
			t.Fatalf("List after: %v", err)
		}
		// Two separate calls → two separate snapshots.
		if len(afterSnaps) != initialCount+2 {
			t.Errorf("snapshot count: got %d, want %d (two calls = two commits)", len(afterSnaps), initialCount+2)
		}

		loaded, err := rules.Load(dir)
		if err != nil {
			t.Fatalf("rules.Load: %v", err)
		}
		if len(loaded) != 3 {
			t.Errorf("got %d rules on disk, want 3", len(loaded))
		}
	})
}
