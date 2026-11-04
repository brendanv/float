package ledger_test

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
)

func TestSetAccountClosed(t *testing.T) {
	dir := t.TempDir()
	mainJournal := `include accounts.journal

2026-01-01 (cc000001) Opening
    assets:old:savings    100 USD
    assets:checking        50 USD
    equity:opening
`
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(mainJournal), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte("account assets:checking\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := mustRealHandler(t, dir)

	closedNames := func(t *testing.T) map[string]bool {
		t.Helper()
		resp, err := h.ListAccounts(t.Context(), connect.NewRequest(&floatv1.ListAccountsRequest{}))
		if err != nil {
			t.Fatalf("ListAccounts: %v", err)
		}
		out := map[string]bool{}
		for _, a := range resp.Msg.Accounts {
			if a.Closed {
				out[a.FullName] = true
			}
		}
		return out
	}

	// Close an undeclared parent: it gets declared, and the child inherits.
	if _, err := h.SetAccountClosed(t.Context(), connect.NewRequest(&floatv1.SetAccountClosedRequest{Name: "assets:old", Closed: true})); err != nil {
		t.Fatalf("SetAccountClosed(close): %v", err)
	}
	got := closedNames(t)
	if !got["assets:old"] || !got["assets:old:savings"] || got["assets:checking"] || len(got) != 2 {
		t.Errorf("closed accounts after close = %v, want assets:old and assets:old:savings", got)
	}

	decls, err := h.ListAccountDeclarations(t.Context(), connect.NewRequest(&floatv1.ListAccountDeclarationsRequest{}))
	if err != nil {
		t.Fatalf("ListAccountDeclarations: %v", err)
	}
	var sawOld bool
	for _, d := range decls.Msg.Declarations {
		if d.Name == "assets:old" {
			sawOld = true
			if !d.Closed {
				t.Errorf("declaration assets:old Closed = false, want true")
			}
		} else if d.Closed {
			t.Errorf("declaration %s Closed = true, want false", d.Name)
		}
	}
	if !sawOld {
		t.Errorf("assets:old not declared after closing")
	}

	// Home-page totals query excludes closed postings.
	bal, err := h.GetBalances(t.Context(), connect.NewRequest(&floatv1.GetBalancesRequest{Depth: 1, Query: []string{"not:tag:float-closed"}}))
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	for _, r := range bal.Msg.Report.Rows {
		if r.FullName == "assets" && (len(r.Amounts) != 1 || r.Amounts[0].Quantity != "50") {
			t.Errorf("open assets total = %v, want 50 USD", r.Amounts)
		}
	}

	// Tags list must not leak the hidden tag.
	tags, err := h.ListTags(t.Context(), connect.NewRequest(&floatv1.ListTagsRequest{}))
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	for _, tag := range tags.Msg.Tags {
		if tag == "float-closed" {
			t.Errorf("ListTags leaked float-closed")
		}
	}

	// Reopen.
	if _, err := h.SetAccountClosed(t.Context(), connect.NewRequest(&floatv1.SetAccountClosedRequest{Name: "assets:old", Closed: false})); err != nil {
		t.Fatalf("SetAccountClosed(reopen): %v", err)
	}
	if got := closedNames(t); len(got) != 0 {
		t.Errorf("closed accounts after reopen = %v, want none", got)
	}

}

func TestSetAccountClosed_RequiresName(t *testing.T) {
	h := mustRealHandler(t, t.TempDir())
	_, err := h.SetAccountClosed(t.Context(), connect.NewRequest(&floatv1.SetAccountClosedRequest{Name: "  ", Closed: true}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}
