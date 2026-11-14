package ledger_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/rules"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/templates"
)

// writeRenameFixture writes a data dir whose journal, rules, templates, and
// config all reference "assets:bank checking" (and look-alike accounts that a
// rename of it must leave alone). extraJournal is appended to the month file.
func writeRenameFixture(t *testing.T, extraJournal string) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.journal", "include accounts.journal\ninclude 2026/01.journal\n")
	write("accounts.journal", "account assets:bank checking  ; type: A\naccount assets:bank\n")
	write("2026/01.journal", "2026-01-05 (aa000001) Groceries\n"+
		"    * assets:bank checking   -5 USD\n"+
		"    expenses:food\n"+
		"\n"+
		"2026-01-06 (aa000002) Transfer\n"+
		"    assets:bank  -1 USD\n"+
		"    (assets:bank checking:joint)  1 USD\n"+
		"    assets:bank checking\n"+
		extraJournal)

	if err := rules.Save(dir, []rules.Rule{
		{ID: "r0000001", Pattern: "transfer", Account: "assets:bank checking:joint", MatchAccount: "assets:bank checking", Priority: 1},
		{ID: "r0000002", Pattern: "coffee", Account: "assets:bank", MatchAccount: "assets:bank checkingx", Priority: 2},
	}); err != nil {
		t.Fatal(err)
	}
	if err := templates.Save(dir, []templates.Template{
		{ID: "t0000001", Name: "Groceries", Postings: []templates.TemplatePosting{
			{Account: "expenses:food", Commodity: "USD", DefaultQuantity: "5"},
			{Account: "assets:bank checking"},
		}},
		{ID: "t0000002", Name: "Other", Postings: []templates.TemplatePosting{
			{Account: "assets:bank"},
			{Account: "assets:bank checkingx:sub"},
		}},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Stripe: config.StripeConfig{LinkedAccounts: []config.StripeLinkedAccount{
			{StripeAccountID: "fca_1", HledgerAccount: "assets:bank checking", DisplayName: "Checking"},
			{StripeAccountID: "fca_2", HledgerAccount: "assets:bank", DisplayName: "Bank"},
		}},
		Portfolio: config.PortfolioConfig{ExcludedAccountPrefixes: []string{"assets:bank checking:joint", "assets:bank"}},
	}
	return dir, cfg
}

func renameAccount(t *testing.T, h *serverledger.Handler, oldName, newName string) (*floatv1.RenameAccountResponse, error) {
	t.Helper()
	resp, err := h.RenameAccount(t.Context(), connect.NewRequest(&floatv1.RenameAccountRequest{OldName: oldName, NewName: newName}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestRenameAccount_UpdatesJournalRulesTemplatesAndConfig(t *testing.T) {
	dir, cfg := writeRenameFixture(t, "")
	h := mustHandlerWithConfig(t, dir, cfg)

	resp, err := renameAccount(t, h, "assets:bank checking", "assets:chase")
	if err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}
	if resp.PostingsRenamed != 3 {
		t.Errorf("PostingsRenamed = %d, want 3", resp.PostingsRenamed)
	}

	journalData, err := os.ReadFile(filepath.Join(dir, "2026", "01.journal"))
	if err != nil {
		t.Fatal(err)
	}
	wantJournal := "2026-01-05 (aa000001) Groceries\n" +
		"    * assets:chase   -5 USD\n" +
		"    expenses:food\n" +
		"\n" +
		"2026-01-06 (aa000002) Transfer\n" +
		"    assets:bank  -1 USD\n" +
		"    (assets:chase:joint)  1 USD\n" +
		"    assets:chase\n"
	if string(journalData) != wantJournal {
		t.Errorf("journal =\n%s\nwant\n%s", journalData, wantJournal)
	}
	declData, err := os.ReadFile(filepath.Join(dir, "accounts.journal"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "account assets:chase  ; type: A\naccount assets:bank\n"; string(declData) != want {
		t.Errorf("accounts.journal = %q, want %q", declData, want)
	}

	gotRules, err := rules.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantRules := [][2]string{
		{"assets:chase:joint", "assets:chase"},
		{"assets:bank", "assets:bank checkingx"},
	}
	if len(gotRules) != len(wantRules) {
		t.Fatalf("got %d rules, want %d", len(gotRules), len(wantRules))
	}
	for i, w := range wantRules {
		if gotRules[i].Account != w[0] || gotRules[i].MatchAccount != w[1] {
			t.Errorf("rule %d account/match_account = %q/%q, want %q/%q", i, gotRules[i].Account, gotRules[i].MatchAccount, w[0], w[1])
		}
	}

	gotTemplates, err := templates.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var gotTemplateAccounts [][]string
	for _, tmpl := range gotTemplates {
		var accts []string
		for _, p := range tmpl.Postings {
			accts = append(accts, p.Account)
		}
		gotTemplateAccounts = append(gotTemplateAccounts, accts)
	}
	wantTemplateAccounts := [][]string{
		{"expenses:food", "assets:chase"},
		{"assets:bank", "assets:bank checkingx:sub"},
	}
	if !reflect.DeepEqual(gotTemplateAccounts, wantTemplateAccounts) {
		t.Errorf("template accounts = %q, want %q", gotTemplateAccounts, wantTemplateAccounts)
	}

	wantLinked := []string{"assets:chase", "assets:bank"}
	wantExcluded := []string{"assets:chase:joint", "assets:bank"}
	checkCfg := func(label string, c *config.Config) {
		t.Helper()
		var linked []string
		for _, la := range c.Stripe.LinkedAccounts {
			linked = append(linked, la.HledgerAccount)
		}
		if !reflect.DeepEqual(linked, wantLinked) {
			t.Errorf("%s linked accounts = %q, want %q", label, linked, wantLinked)
		}
		if !reflect.DeepEqual(c.Portfolio.ExcludedAccountPrefixes, wantExcluded) {
			t.Errorf("%s excluded prefixes = %q, want %q", label, c.Portfolio.ExcludedAccountPrefixes, wantExcluded)
		}
	}
	savedCfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	checkCfg("saved", savedCfg)
	checkCfg("in-memory", serverledger.ExportedLoadCfg(h))

	// The original config value is shared copy-on-write state and must not
	// have been mutated in place.
	if cfg.Stripe.LinkedAccounts[0].HledgerAccount != "assets:bank checking" {
		t.Errorf("original config mutated: %q", cfg.Stripe.LinkedAccounts[0].HledgerAccount)
	}
}

func TestRenameAccount_CheckFailureRevertsEverything(t *testing.T) {
	// The assertion on assets:chase holds today, but merging the earlier
	// assets:bank checking postings into it makes the assertion fail, so
	// hledger check rejects the rename and everything must be reverted.
	dir, cfg := writeRenameFixture(t, "\n2026-01-20 (aa000003) Deposit\n"+
		"    assets:chase  10 USD = 10 USD\n"+
		"    income:salary\n")
	h := mustHandlerWithConfig(t, dir, cfg)

	snapshot := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, rel := range []string{"2026/01.journal", "accounts.journal", "rules.json", "templates.json", "config.toml"} {
			data, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				t.Fatal(err)
			}
			out[rel] = string(data)
		}
		return out
	}
	before := snapshot()
	cfgBefore := serverledger.ExportedLoadCfg(h)

	if _, err := renameAccount(t, h, "assets:bank checking", "assets:chase"); err == nil {
		t.Fatal("RenameAccount succeeded, want hledger check failure")
	}

	after := snapshot()
	for rel, want := range before {
		if after[rel] != want {
			t.Errorf("%s changed after failed rename:\n%s\nwant\n%s", rel, after[rel], want)
		}
	}
	if got := serverledger.ExportedLoadCfg(h); got != cfgBefore {
		t.Errorf("in-memory config replaced after failed rename: linked = %+v", got.Stripe.LinkedAccounts)
	}
}

func TestRenameAccount_NoConfigReferencesLeavesFilesAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("2026-01-01 (aa000001) x\n    assets:a  1 USD\n    equity\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	if _, err := renameAccount(t, h, "assets:a", "assets:b"); err != nil {
		t.Fatalf("RenameAccount: %v", err)
	}
	for _, name := range []string{"rules.json", "templates.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was created by a rename that touched no rules/templates (stat err %v)", name, err)
		}
	}
}
