package txfilter_test

import (
	"path/filepath"
	"testing"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/txfilter"
)

func mustClient(t *testing.T) *hledger.Client {
	t.Helper()
	c, err := hledger.New("hledger", filepath.Join("testdata", "filter.journal"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// fids returns the sorted set of FIDs from a transaction slice, for
// order-independent comparison.
func fids(txns []hledger.Transaction) map[string]bool {
	out := make(map[string]bool, len(txns))
	for _, t := range txns {
		out[t.FID] = true
	}
	return out
}

func TestEquivalenceWithHledger(t *testing.T) {
	c := mustClient(t)
	ctx := t.Context()

	all, err := c.Transactions(ctx)
	if err != nil {
		t.Fatalf("Transactions(all): %v", err)
	}

	tests := []struct {
		name  string
		query []string
	}{
		{"no filter", nil},
		{"date range", []string{"date:2026/01/10..2026/02/02"}},
		{"date range hyphenated", []string{"date:2026-01-10..2026-02-02"}},
		{"date open start", []string{"date:..2026/01/16"}},
		{"date open end", []string{"date:2026/02/01.."}},
		{"date exact", []string{"date:2026/01/20"}},
		{"date ignores posting override", []string{"date:2026/02/08..2026/02/12"}},
		{"acct infix", []string{"acct:shopping"}},
		{"acct case insensitive", []string{"acct:SHOPPING"}},
		{"acct regex anchor", []string{"acct:^expenses"}},
		{"acct regex dot metachar", []string{"acct:exp.nses"}},
		{"payee match", []string{"payee:Amazon"}},
		{"payee excludes note part", []string{"payee:123"}},
		{"desc match note part", []string{"desc:123"}},
		{"desc regex metachar paren", []string{`desc:Whole Foods \(Market\)`}},
		{"desc case insensitive", []string{"desc:payroll"}},
		{"tag key only, txn-level", []string{"tag:posted"}},
		{"tag key only, posting-level", []string{"tag:category"}},
		{"tag key=value regex", []string{"tag:other=bar"}},
		{"tag key=value no match", []string{"tag:other=zzz"}},
		{"status cleared", []string{"status:*"}},
		{"status pending", []string{"status:!"}},
		{"status unmarked", []string{"status:"}},
		{"not status cleared", []string{"not:status:*"}},
		{"not desc", []string{"not:desc:payroll"}},
		{"not acct", []string{"not:acct:shopping"}},
		{"combined acct and date", []string{"acct:checking", "date:2026/01/01..2026/02/01"}},
		{"combined payee and status", []string{"payee:Amazon", "status:*"}},
		{"code exact", []string{"code:aa001100"}},
		{"tag key substring", []string{"tag:ost"}},
		{"code substring", []string{"code:001"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, err := c.Transactions(ctx, tt.query...)
			if err != nil {
				t.Fatalf("hledger Transactions(%v): %v", tt.query, err)
			}

			f, ok := txfilter.Parse(tt.query)
			if !ok {
				t.Fatalf("Parse(%v) returned ok=false, expected a supported query", tt.query)
			}
			got := f.Filter(all)

			wantFIDs, gotFIDs := fids(want), fids(got)
			if len(wantFIDs) != len(gotFIDs) {
				t.Fatalf("query %v: hledger matched %v, txfilter matched %v", tt.query, wantFIDs, gotFIDs)
			}
			for fid := range wantFIDs {
				if !gotFIDs[fid] {
					t.Errorf("query %v: hledger matched %s but txfilter did not (hledger=%v, txfilter=%v)", tt.query, fid, wantFIDs, gotFIDs)
				}
			}
		})
	}
}

func TestParseUnsupportedTokensFallBack(t *testing.T) {
	unsupported := [][]string{
		{"depth:2"},
		{"amt:>100"},
		{"real:1"},
		{"cur:USD"},
		{"inacct:assets"},
		{"date:notadate"},
		{"acct:("}, // invalid regex
		{"noColon"},
	}
	for _, tokens := range unsupported {
		t.Run(tokens[0], func(t *testing.T) {
			if _, ok := txfilter.Parse(tokens); ok {
				t.Errorf("Parse(%v) = ok, want fallback (ok=false)", tokens)
			}
		})
	}
}

func TestMixedSupportedAndUnsupportedFallsBack(t *testing.T) {
	if _, ok := txfilter.Parse([]string{"acct:checking", "depth:2"}); ok {
		t.Error("Parse with one unsupported token among supported ones should return ok=false")
	}
}

// TestRepeatedKeywordFallsBack confirms Parse rejects repeated same-type
// positive keyword tokens rather than silently ANDing them: hledger ORs
// repeated terms of the same type (e.g. "desc:a desc:b" matches either), but
// Filter.Match's flat predicate list can only express AND. Falling back lets
// the caller's cachedTransactions/cachedAregister path query hledger
// directly instead.
func TestRepeatedKeywordFallsBack(t *testing.T) {
	repeated := [][]string{
		{"desc:amazon", "desc:payroll"},
		{"acct:shopping", "acct:salary"},
		{"tag:food", "tag:travel"},
	}
	for _, tokens := range repeated {
		t.Run(tokens[0], func(t *testing.T) {
			if _, ok := txfilter.Parse(tokens); ok {
				t.Errorf("Parse(%v) = ok, want fallback (ok=false) for repeated keyword", tokens)
			}
		})
	}
}

// TestRepeatedKeywordUnionEquivalence documents why TestRepeatedKeywordFallsBack
// exists: hledger really does OR repeated same-type terms, which a flat
// AND-of-predicates Filter cannot express.
func TestRepeatedKeywordUnionEquivalence(t *testing.T) {
	c := mustClient(t)
	ctx := t.Context()

	union, err := c.Transactions(ctx, "desc:amazon", "desc:payroll")
	if err != nil {
		t.Fatalf("hledger Transactions: %v", err)
	}
	amazonOnly, err := c.Transactions(ctx, "desc:amazon")
	if err != nil {
		t.Fatalf("hledger Transactions: %v", err)
	}
	if len(union) <= len(amazonOnly) {
		t.Fatalf("expected hledger to OR repeated desc: terms (union %d > desc:amazon alone %d)", len(union), len(amazonOnly))
	}
}

// TestTagSubstringNotExactEquivalence confirms hledger's tag: key matching is
// an infix regex, not exact equality: a fixture tagged "posted" is also
// matched by "tag:ost" (README finding #5).
func TestTagSubstringNotExactEquivalence(t *testing.T) {
	c := mustClient(t)
	ctx := t.Context()

	exact, err := c.Transactions(ctx, "tag:posted")
	if err != nil {
		t.Fatalf("hledger Transactions: %v", err)
	}
	substr, err := c.Transactions(ctx, "tag:ost")
	if err != nil {
		t.Fatalf("hledger Transactions: %v", err)
	}
	if len(exact) != len(substr) {
		t.Fatalf("expected tag:posted and tag:ost to match the same set via infix regex, got %d vs %d", len(exact), len(substr))
	}
}
