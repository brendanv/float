package ledger_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	serverledger "github.com/brendanv/float/internal/server/ledger"
)

// transfersJournal has a matched card payment, two old lone sides (one of
// them a duplicate savings transfer) and a far-future side that is still
// within its window and so only "recent".
const transfersJournal = `2026-01-05 (tt000001) ONLINE PMT CARD
    assets:checking          -500.00 USD
    equity:transfers

2026-01-06 (tt000002) PAYMENT THANK YOU
    liabilities:card          500.00 USD
    equity:transfers

2026-01-10 (tt000003) XFER TO SAVINGS
    assets:checking          -100.00 USD
    equity:transfers

2026-01-11 (tt000004) XFER FROM CHECKING
    assets:savings            100.00 USD
    equity:transfers

2026-01-24 (tt000005) XFER TO SAVINGS
    assets:checking          -100.00 USD
    equity:transfers

2026-02-03 (tt000006) Venmo
    assets:venmo              -20.00 USD
    equity:transfers

2099-01-01 (tt000007) Scheduled XFER
    assets:checking           -75.00 USD
    equity:transfers
`

func writeTransfersJournal(t *testing.T, journal string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func findUnmatched(t *testing.T, h *serverledger.Handler, fromDate string) *floatv1.FindUnmatchedTransfersResponse {
	t.Helper()
	resp, err := h.FindUnmatchedTransfers(t.Context(), connect.NewRequest(&floatv1.FindUnmatchedTransfersRequest{FromDate: fromDate}))
	if err != nil {
		t.Fatalf("FindUnmatchedTransfers: %v", err)
	}
	return resp.Msg
}

func unmatchedSummary(resp *floatv1.FindUnmatchedTransfersResponse) []string {
	out := make([]string, len(resp.Transfers))
	for i, u := range resp.Transfers {
		out[i] = u.Transaction.Fid + ":" + u.Status + ":" + u.Amount.Quantity + " " + u.Amount.Commodity
	}
	return out
}

func TestFindUnmatchedTransfers(t *testing.T) {
	h := mustRealHandler(t, writeTransfersJournal(t, transfersJournal))

	t.Run("all history, newest first", func(t *testing.T) {
		resp := findUnmatched(t, h, "")
		want := []string{
			"tt000007:recent:75.00 USD",
			"tt000006:missing:20.00 USD",
			"tt000005:missing:100.00 USD",
		}
		if got := unmatchedSummary(resp); !slices.Equal(got, want) {
			t.Errorf("transfers = %v, want %v", got, want)
		}
		if resp.Account != "equity:transfers" {
			t.Errorf("account = %q, want equity:transfers", resp.Account)
		}
		if resp.WindowDays != 5 {
			t.Errorf("window_days = %d, want 5", resp.WindowDays)
		}
	})

	t.Run("from_date filters results, not matching", func(t *testing.T) {
		// tt000002 is on/after the cutoff but still pairs with tt000001.
		resp := findUnmatched(t, h, "2026-01-06")
		want := []string{
			"tt000007:recent:75.00 USD",
			"tt000006:missing:20.00 USD",
			"tt000005:missing:100.00 USD",
		}
		if got := unmatchedSummary(resp); !slices.Equal(got, want) {
			t.Errorf("transfers = %v, want %v", got, want)
		}
		resp = findUnmatched(t, h, "2026-02-01")
		if got, want := unmatchedSummary(resp), want[:2]; !slices.Equal(got, want) {
			t.Errorf("transfers = %v, want %v", got, want)
		}
	})

	t.Run("invalid from_date", func(t *testing.T) {
		_, err := h.FindUnmatchedTransfers(t.Context(), connect.NewRequest(&floatv1.FindUnmatchedTransfersRequest{FromDate: "Jan 1"}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})
}

func TestFindUnmatchedTransfers_ConfiguredAccount(t *testing.T) {
	dir := writeTransfersJournal(t, `2026-01-05 (tt000001) ONLINE PMT CARD
    assets:checking          -500.00 USD
    assets:in transit (2)

2026-01-07 (tt000002) Lone side via default account
    assets:checking          -1.00 USD
    equity:transfers
`)
	h := mustHandlerWithConfig(t, dir, &config.Config{Transfers: config.TransfersConfig{Account: "assets:in transit (2)"}})

	resp := findUnmatched(t, h, "")
	if got, want := unmatchedSummary(resp), []string{"tt000001:missing:500.00 USD"}; !slices.Equal(got, want) {
		t.Errorf("transfers = %v, want %v", got, want)
	}
	if resp.Account != "assets:in transit (2)" {
		t.Errorf("account = %q, want assets:in transit (2)", resp.Account)
	}
	if got, want := routesSummary(resp), []string{"assets:in transit (2) [assets:checking] [500.00 USD]"}; !slices.Equal(got, want) {
		t.Errorf("routes = %q, want %q", got, want)
	}
}

// Renaming the default clearing account while config leaves it unset must
// point the check at the new name.
func TestRenameAccount_RenamesDefaultTransfersAccount(t *testing.T) {
	// RenameAccount rewrites the month files float manages, so use that layout.
	dir := writeTransfersJournal(t, "include 2026/01.journal\n")
	if err := os.MkdirAll(filepath.Join(dir, "2026"), 0o755); err != nil {
		t.Fatal(err)
	}
	month := "2026-01-05 (tt000001) ONLINE PMT CARD\n    assets:checking  -500.00 USD\n    equity:transfers\n"
	if err := os.WriteFile(filepath.Join(dir, "2026", "01.journal"), []byte(month), 0o644); err != nil {
		t.Fatal(err)
	}
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	if _, err := renameAccount(t, h, "equity:transfers", "equity:clearing"); err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}
	saved, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Transfers.Account != "equity:clearing" {
		t.Errorf("saved transfers.account = %q, want equity:clearing", saved.Transfers.Account)
	}
	resp := findUnmatched(t, h, "")
	if resp.Account != "equity:clearing" {
		t.Errorf("account = %q, want equity:clearing", resp.Account)
	}
	if got, want := unmatchedSummary(resp), []string{"tt000001:missing:500.00 USD"}; !slices.Equal(got, want) {
		t.Errorf("transfers = %v, want %v", got, want)
	}
}

func routesSummary(resp *floatv1.FindUnmatchedTransfersResponse) []string {
	out := make([]string, len(resp.Routes))
	for i, r := range resp.Routes {
		var bal []string
		for _, a := range r.Balance {
			bal = append(bal, a.Quantity+" "+a.Commodity)
		}
		out[i] = fmt.Sprintf("%s %v %v", r.Account, r.Accounts, bal)
	}
	return out
}

// Routes (direct children of equity:transfers) scope matching, name where the
// other side was expected, and carry hledger's balance of the route.
func TestFindUnmatchedTransfers_Routes(t *testing.T) {
	h := mustRealHandler(t, writeTransfersJournal(t, `2026-01-05 (tt000001) CHASE PMT SAPPHIRE
    assets:checking                -500.00 USD
    equity:transfers:Chase-Cards

2026-01-06 (tt000002) SAPPHIRE PAYMENT
    liabilities:sapphire            500.00 USD
    equity:transfers:Chase-Cards

2026-01-10 (tt000003) CHASE PMT FREEDOM
    assets:checking                -200.00 USD
    equity:transfers:Chase-Cards

2026-01-11 (tt000004) FREEDOM PAYMENT
    liabilities:freedom             200.00 USD
    equity:transfers:Chase-Cards

2026-02-01 (tt000005) SAPPHIRE PAYMENT
    liabilities:sapphire            300.00 USD
    equity:transfers:Chase-Cards

; Same amount and dates as tt000005 but another route: no match.
2026-02-01 (tt000006) SCHWAB TO AMEX
    assets:schwab                  -300.00 USD
    equity:transfers:Schwab-Amex:pending

2026-02-02 (tt000007) Fully matched savings move
    assets:checking                 -40.00 USD
    equity:transfers:Chase-Savings

2026-02-02 (tt000008) Fully matched savings move
    assets:savings                   40.00 USD
    equity:transfers:Chase-Savings
`))

	resp := findUnmatched(t, h, "")
	var got []string
	for _, u := range resp.Transfers {
		got = append(got, fmt.Sprintf("%s %s %v", u.Transaction.Fid, u.Route, u.ExpectedAccounts))
	}
	want := []string{
		// The route has only ever been used by assets:schwab: nothing to expect.
		"tt000006 equity:transfers:Schwab-Amex []",
		// A lone card payment points at checking, not the other card.
		"tt000005 equity:transfers:Chase-Cards [assets:checking]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("transfers = %q, want %q", got, want)
	}

	wantRoutes := []string{
		"equity:transfers:Chase-Cards [assets:checking liabilities:freedom liabilities:sapphire] [-300.00 USD]",
		"equity:transfers:Schwab-Amex [assets:schwab] [300.00 USD]",
	}
	if got := routesSummary(resp); !slices.Equal(got, wantRoutes) {
		t.Errorf("routes = %q, want %q", got, wantRoutes)
	}

	// Routes follow from_date: only routes with a returned side are listed.
	if got := routesSummary(findUnmatched(t, h, "2026-02-02")); len(got) != 0 {
		t.Errorf("routes after from_date = %q, want none", got)
	}
}
