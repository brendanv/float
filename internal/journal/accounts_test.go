package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/testgen"
)

func TestListAccountDeclarations(t *testing.T) {
	t.Run("no_accounts_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 100, NumTxns: 1})
		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		if len(decls) != 0 {
			t.Errorf("expected empty slice, got %d declarations", len(decls))
		}
	})

	t.Run("parses_directives", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: account declarations\n" +
			"account assets:checking  ; aid:a1b2c3d4\n" +
			"account expenses:food  ; aid:e5f6a7b8\n" +
			"account income:salary\n" // no AID
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		if len(decls) != 3 {
			t.Fatalf("expected 3 declarations, got %d", len(decls))
		}

		tests := []struct {
			idx  int
			name string
			aid  string
		}{
			{0, "assets:checking", "a1b2c3d4"},
			{1, "expenses:food", "e5f6a7b8"},
			{2, "income:salary", ""},
		}
		for _, tt := range tests {
			d := decls[tt.idx]
			if d.Name != tt.name {
				t.Errorf("[%d] Name = %q, want %q", tt.idx, d.Name, tt.name)
			}
			if d.AID != tt.aid {
				t.Errorf("[%d] AID = %q, want %q", tt.idx, d.AID, tt.aid)
			}
		}
	})
}

func TestAppendAccountDeclaration(t *testing.T) {
	t.Run("creates_accounts_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 110, NumTxns: 1})

		aid, err := AppendAccountDeclaration(dir, "assets:newaccount")
		if err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}
		if len(aid) != 8 {
			t.Errorf("expected 8-char aid, got %q", aid)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		if len(decls) != 1 {
			t.Fatalf("expected 1 declaration, got %d", len(decls))
		}
		d := decls[0]
		if d.AID != aid {
			t.Errorf("AID = %q, want %q", d.AID, aid)
		}
		if d.Name != "assets:newaccount" {
			t.Errorf("Name = %q, want assets:newaccount", d.Name)
		}
	})

	t.Run("include_prepended_in_main", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 111, NumTxns: 1})

		if _, err := AppendAccountDeclaration(dir, "assets:newaccount"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		lines := strings.Split(string(mainData), "\n")

		accountsIdx, monthIdx := -1, -1
		for i, line := range lines {
			if strings.TrimSpace(line) == "include accounts.journal" {
				accountsIdx = i
			}
			if strings.HasPrefix(strings.TrimSpace(line), "include 20") && monthIdx == -1 {
				monthIdx = i
			}
		}
		if accountsIdx == -1 {
			t.Fatal("include accounts.journal not found in main.journal")
		}
		if monthIdx != -1 && accountsIdx > monthIdx {
			t.Errorf("accounts include (line %d) should appear before month includes (line %d)", accountsIdx, monthIdx)
		}
	})

	t.Run("idempotent_main_include", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 112, NumTxns: 1})

		for i := range 3 {
			if _, err := AppendAccountDeclaration(dir, "assets:newaccount"); err != nil {
				t.Fatalf("AppendAccountDeclaration #%d: %v", i+1, err)
			}
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		count := strings.Count(string(mainData), "include accounts.journal")
		if count != 1 {
			t.Errorf("expected 1 accounts include, found %d", count)
		}
	})

	t.Run("include_before_prices", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 113, NumTxns: 1})

		// Add a price first (so prices include gets prepended).
		if _, err := AppendPrice(dir, "2026-01-01", "BTC", "45000.00", "USD"); err != nil {
			t.Fatalf("AppendPrice: %v", err)
		}

		// Now declare an account — accounts include must still come before prices.
		if _, err := AppendAccountDeclaration(dir, "assets:btc"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		lines := strings.Split(string(mainData), "\n")

		accountsIdx, pricesIdx := -1, -1
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "include accounts.journal" {
				accountsIdx = i
			}
			if trimmed == "include prices.journal" {
				pricesIdx = i
			}
		}
		if accountsIdx == -1 {
			t.Fatal("include accounts.journal not found in main.journal")
		}
		if pricesIdx == -1 {
			t.Fatal("include prices.journal not found in main.journal")
		}
		if accountsIdx > pricesIdx {
			t.Errorf("accounts include (line %d) should appear before prices include (line %d)", accountsIdx, pricesIdx)
		}
	})

	t.Run("prices_after_accounts_when_price_added_after", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 114, NumTxns: 1})

		// Add account first.
		if _, err := AppendAccountDeclaration(dir, "assets:btc"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		// Now add a price — prices include must come after accounts.
		if _, err := AppendPrice(dir, "2026-01-01", "BTC", "45000.00", "USD"); err != nil {
			t.Fatalf("AppendPrice: %v", err)
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatalf("read main.journal: %v", err)
		}
		lines := strings.Split(string(mainData), "\n")

		accountsIdx, pricesIdx := -1, -1
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "include accounts.journal" {
				accountsIdx = i
			}
			if trimmed == "include prices.journal" {
				pricesIdx = i
			}
		}
		if accountsIdx == -1 {
			t.Fatal("include accounts.journal not found in main.journal")
		}
		if pricesIdx == -1 {
			t.Fatal("include prices.journal not found in main.journal")
		}
		if accountsIdx > pricesIdx {
			t.Errorf("accounts include (line %d) should appear before prices include (line %d)", accountsIdx, pricesIdx)
		}
	})
}

func TestDeleteAccountDeclaration(t *testing.T) {
	t.Run("removes_declaration", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 120, NumTxns: 1})

		aid1, err := AppendAccountDeclaration(dir, "assets:checking")
		if err != nil {
			t.Fatalf("AppendAccountDeclaration 1: %v", err)
		}
		aid2, err := AppendAccountDeclaration(dir, "assets:savings")
		if err != nil {
			t.Fatalf("AppendAccountDeclaration 2: %v", err)
		}

		if err := DeleteAccountDeclaration(dir, aid1); err != nil {
			t.Fatalf("DeleteAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations after delete: %v", err)
		}
		if len(decls) != 1 {
			t.Fatalf("expected 1 declaration after delete, got %d", len(decls))
		}
		if decls[0].AID != aid2 {
			t.Errorf("remaining AID = %q, want %q", decls[0].AID, aid2)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 121, NumTxns: 1})

		if _, err := AppendAccountDeclaration(dir, "assets:checking"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		err := DeleteAccountDeclaration(dir, "00000000")
		if err == nil {
			t.Fatal("expected error for non-existent aid, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("no_accounts_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 122, NumTxns: 1})

		err := DeleteAccountDeclaration(dir, "a1b2c3d4")
		if err == nil {
			t.Fatal("expected error when accounts.journal absent, got nil")
		}
	})
}

func TestEnsureAccountsInclude(t *testing.T) {
	t.Run("idempotent", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 130, NumTxns: 1})

		for i := range 3 {
			if err := EnsureAccountsInclude(dir); err != nil {
				t.Fatalf("EnsureAccountsInclude #%d: %v", i+1, err)
			}
		}

		mainData, err := os.ReadFile(filepath.Join(dir, "main.journal"))
		if err != nil {
			t.Fatal(err)
		}
		count := strings.Count(string(mainData), "include accounts.journal")
		if count != 1 {
			t.Errorf("expected 1 accounts include, found %d", count)
		}
	})
}
