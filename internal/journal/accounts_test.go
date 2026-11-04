package journal

import (
	"errors"
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
		// Legacy files with aid comments are silently ignored (backward compat).
		content := "; float: account declarations\n" +
			"account assets:checking  ; aid:a1b2c3d4\n" +
			"account expenses:food  ; aid:e5f6a7b8\n" +
			"account income:salary\n"
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

		want := []string{"assets:checking", "expenses:food", "income:salary"}
		for i, name := range want {
			if decls[i].Name != name {
				t.Errorf("[%d] Name = %q, want %q", i, decls[i].Name, name)
			}
		}
	})
}

func TestAppendAccountDeclaration(t *testing.T) {
	t.Run("creates_accounts_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 110, NumTxns: 1})

		if err := AppendAccountDeclaration(dir, "assets:newaccount"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		if len(decls) != 1 {
			t.Fatalf("expected 1 declaration, got %d", len(decls))
		}
		if decls[0].Name != "assets:newaccount" {
			t.Errorf("Name = %q, want assets:newaccount", decls[0].Name)
		}
	})

	t.Run("no_aid_in_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 115, NumTxns: 1})

		if err := AppendAccountDeclaration(dir, "assets:checking"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "accounts.journal"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "aid:") {
			t.Errorf("accounts.journal should not contain aid comments, got:\n%s", data)
		}
	})

	t.Run("include_prepended_in_main", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 111, NumTxns: 1})

		if err := AppendAccountDeclaration(dir, "assets:newaccount"); err != nil {
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
			if err := AppendAccountDeclaration(dir, "assets:newaccount"); err != nil {
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
		if err := AppendAccountDeclaration(dir, "assets:btc"); err != nil {
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
		if err := AppendAccountDeclaration(dir, "assets:btc"); err != nil {
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

		if err := AppendAccountDeclaration(dir, "assets:checking"); err != nil {
			t.Fatalf("AppendAccountDeclaration 1: %v", err)
		}
		if err := AppendAccountDeclaration(dir, "assets:savings"); err != nil {
			t.Fatalf("AppendAccountDeclaration 2: %v", err)
		}

		if err := DeleteAccountDeclaration(dir, "assets:checking"); err != nil {
			t.Fatalf("DeleteAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations after delete: %v", err)
		}
		if len(decls) != 1 {
			t.Fatalf("expected 1 declaration after delete, got %d", len(decls))
		}
		if decls[0].Name != "assets:savings" {
			t.Errorf("remaining Name = %q, want assets:savings", decls[0].Name)
		}
	})

	t.Run("deletes_legacy_aid_line", func(t *testing.T) {
		// Backward compat: delete works on lines that still have the old ; aid:xxx comment.
		dir := t.TempDir()
		content := "; float: account declarations\n" +
			"account assets:checking  ; aid:a1b2c3d4\n" +
			"account assets:savings\n"
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := DeleteAccountDeclaration(dir, "assets:checking"); err != nil {
			t.Fatalf("DeleteAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations after delete: %v", err)
		}
		if len(decls) != 1 || decls[0].Name != "assets:savings" {
			t.Errorf("expected [assets:savings], got %v", decls)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 121, NumTxns: 1})

		if err := AppendAccountDeclaration(dir, "assets:checking"); err != nil {
			t.Fatalf("AppendAccountDeclaration: %v", err)
		}

		err := DeleteAccountDeclaration(dir, "assets:nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent account, got nil")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("no_accounts_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 122, NumTxns: 1})

		err := DeleteAccountDeclaration(dir, "assets:checking")
		if err == nil {
			t.Fatal("expected error when accounts.journal absent, got nil")
		}
	})
}

func TestEnsureAccountsInclude(t *testing.T) {
	t.Run("idempotent", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 130, NumTxns: 1})

		for i := range 3 {
			if _, err := EnsureAccountsInclude(dir); err != nil {
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

func TestSetClosedTag(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		closed bool
		want   string
	}{
		{name: "close bare", line: "account assets:old", closed: true, want: "account assets:old  ; float-closed:"},
		{name: "close with type tag", line: "account assets:old  ; type: A", closed: true, want: "account assets:old  ; type: A, float-closed:"},
		{name: "close already closed", line: "account assets:old  ; float-closed:", closed: true, want: "account assets:old  ; float-closed:"},
		{name: "reopen only tag", line: "account assets:old  ; float-closed:", closed: false, want: "account assets:old"},
		{name: "reopen keeps other tags", line: "account assets:old  ; type: A, float-closed:", closed: false, want: "account assets:old  ; type: A"},
		{name: "reopen tag first", line: "account assets:old  ; float-closed:, type: A", closed: false, want: "account assets:old  ; type: A"},
		{name: "reopen open account", line: "account assets:old  ; type: A", closed: false, want: "account assets:old  ; type: A"},
		{name: "reopen bare", line: "account assets:old", closed: false, want: "account assets:old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setClosedTag(tt.line, tt.closed); got != tt.want {
				t.Errorf("setClosedTag(%q, %v) = %q, want %q", tt.line, tt.closed, got, tt.want)
			}
		})
	}
}

func TestSetAccountClosed(t *testing.T) {
	writeAccounts := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte("include accounts.journal\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	readAccounts := func(t *testing.T, dir string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, "accounts.journal"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	t.Run("close and reopen declared account", func(t *testing.T) {
		dir := writeAccounts(t, "account assets:checking\naccount assets:old  ; type: A\n")
		if err := SetAccountClosed(dir, "assets:old", true); err != nil {
			t.Fatal(err)
		}
		if got, want := readAccounts(t, dir), "account assets:checking\naccount assets:old  ; type: A, float-closed:\n"; got != want {
			t.Errorf("after close:\n%q\nwant\n%q", got, want)
		}
		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatal(err)
		}
		if decls[0].Closed || !decls[1].Closed {
			t.Errorf("Closed flags = %v, %v; want false, true", decls[0].Closed, decls[1].Closed)
		}
		if err := SetAccountClosed(dir, "assets:old", false); err != nil {
			t.Fatal(err)
		}
		if got, want := readAccounts(t, dir), "account assets:checking\naccount assets:old  ; type: A\n"; got != want {
			t.Errorf("after reopen:\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("close undeclared account declares it", func(t *testing.T) {
		dir := writeAccounts(t, "account assets:checking\n")
		if err := SetAccountClosed(dir, "assets:old", true); err != nil {
			t.Fatal(err)
		}
		if got, want := readAccounts(t, dir), "account assets:checking\naccount assets:old  ; float-closed:\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("reopen undeclared account is a no-op", func(t *testing.T) {
		dir := writeAccounts(t, "account assets:checking\n")
		if err := SetAccountClosed(dir, "assets:old", false); err != nil {
			t.Fatal(err)
		}
		if got, want := readAccounts(t, dir), "account assets:checking\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("rename preserves closed tag", func(t *testing.T) {
		dir := writeAccounts(t, "account assets:old  ; float-closed:\naccount assets:old:sub\n")
		if err := RenameAccountDeclaration(dir, "assets:old", "assets:legacy"); err != nil {
			t.Fatal(err)
		}
		if got, want := readAccounts(t, dir), "account assets:legacy  ; float-closed:\naccount assets:legacy:sub\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
