package journal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/testgen"
)

func TestRenameAccountDeclaration(t *testing.T) {
	t.Run("renames_exact_match", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: account declarations\n" +
			"account assets:checking\n" +
			"account assets:savings\n" +
			"account expenses:food\n"
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := RenameAccountDeclaration(dir, "assets:checking", "assets:bank"); err != nil {
			t.Fatalf("RenameAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		names := make([]string, len(decls))
		for i, d := range decls {
			names[i] = d.Name
		}
		want := []string{"assets:bank", "assets:savings", "expenses:food"}
		for i, w := range want {
			if names[i] != w {
				t.Errorf("[%d] got %q, want %q", i, names[i], w)
			}
		}
	})

	t.Run("renames_sub_accounts", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: account declarations\n" +
			"account assets:checking\n" +
			"account assets:checking:joint\n" +
			"account assets:savings\n"
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := RenameAccountDeclaration(dir, "assets:checking", "assets:bank"); err != nil {
			t.Fatalf("RenameAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		names := make([]string, len(decls))
		for i, d := range decls {
			names[i] = d.Name
		}
		want := []string{"assets:bank", "assets:bank:joint", "assets:savings"}
		for i, w := range want {
			if names[i] != w {
				t.Errorf("[%d] got %q, want %q", i, names[i], w)
			}
		}
	})

	t.Run("does_not_rename_prefix_only_match", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: account declarations\n" +
			"account assets:check\n" +
			"account assets:checking\n"
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := RenameAccountDeclaration(dir, "assets:check", "assets:verify"); err != nil {
			t.Fatalf("RenameAccountDeclaration: %v", err)
		}

		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		// assets:check → assets:verify; assets:checking must be unchanged.
		if decls[0].Name != "assets:verify" {
			t.Errorf("got %q, want assets:verify", decls[0].Name)
		}
		if decls[1].Name != "assets:checking" {
			t.Errorf("got %q, want assets:checking", decls[1].Name)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: account declarations\naccount assets:savings\n"
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		err := RenameAccountDeclaration(dir, "assets:nonexistent", "assets:other")
		if err == nil {
			t.Fatal("expected error for non-existent account, got nil")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("no_accounts_file", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 200, NumTxns: 1})

		err := RenameAccountDeclaration(dir, "assets:checking", "assets:bank")
		if err == nil {
			t.Fatal("expected error when accounts.journal absent, got nil")
		}
	})
}

func TestRenameAccountInPosting(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		oldName string
		newName string
		want    string
		changed bool
	}{
		{
			name:    "exact_match_with_amount",
			line:    "    assets:checking  100.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    assets:bank  100.00 USD",
			changed: true,
		},
		{
			name:    "exact_match_no_amount",
			line:    "    assets:checking",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    assets:bank",
			changed: true,
		},
		{
			name:    "sub_account_renamed",
			line:    "    assets:checking:joint  50.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    assets:bank:joint  50.00 USD",
			changed: true,
		},
		{
			name:    "prefix_only_not_renamed",
			line:    "    assets:checkingaccount  100.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    assets:checkingaccount  100.00 USD",
			changed: false,
		},
		{
			name:    "no_match",
			line:    "    expenses:food  45.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    expenses:food  45.00 USD",
			changed: false,
		},
		{
			name:    "transaction_header_not_touched",
			line:    "2026-01-05 (aa001100) PAYROLL",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "2026-01-05 (aa001100) PAYROLL",
			changed: false,
		},
		{
			name:    "indented_comment_not_touched",
			line:    "    ; float-category:assets:checking",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    ; float-category:assets:checking",
			changed: false,
		},
		{
			name:    "single_space_not_a_posting",
			line:    " assets:checking  100.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    " assets:checking  100.00 USD",
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := renameAccountInPosting(tt.line, tt.oldName, tt.newName)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
		})
	}
}

func TestRenameAccountInJournalFiles(t *testing.T) {
	t.Run("renames_across_month_files", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{
			Seed:    300,
			NumTxns: 6,
			Accounts: []string{
				"assets:checking",
				"expenses:food",
				"income:salary",
			},
		})

		n, err := RenameAccountInJournalFiles(dir, "assets:checking", "assets:bank")
		if err != nil {
			t.Fatalf("RenameAccountInJournalFiles: %v", err)
		}
		if n == 0 {
			t.Error("expected at least one posting renamed, got 0")
		}

		// Verify no remaining occurrences of the old name in any journal file.
		files, _ := filepath.Glob(filepath.Join(dir, "*", "*.journal"))
		for _, f := range files {
			data, _ := os.ReadFile(f)
			if strings.Contains(string(data), "assets:checking") {
				t.Errorf("old account name still present in %s", f)
			}
			if !strings.Contains(string(data), "assets:bank") {
				t.Errorf("new account name not found in %s (expected it there)", f)
			}
		}
	})

	t.Run("no_match_returns_zero", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{
			Seed:    301,
			NumTxns: 3,
			Accounts: []string{"assets:checking", "expenses:food"},
		})

		n, err := RenameAccountInJournalFiles(dir, "assets:nonexistent", "assets:other")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 renames, got %d", n)
		}
	})

	t.Run("sub_accounts_renamed", func(t *testing.T) {
		dir := t.TempDir()
		// Write a month file with sub-account postings.
		monthDir := filepath.Join(dir, "2026")
		if err := os.MkdirAll(monthDir, 0755); err != nil {
			t.Fatal(err)
		}
		content := "; float: 2026/01\n" +
			"2026-01-05 (aa001100) PAYROLL\n" +
			"    assets:checking:joint  3500.00 USD\n" +
			"    income:salary\n" +
			"\n" +
			"2026-01-10 (bb002200) TRANSFER\n" +
			"    assets:checking  -500.00 USD\n" +
			"    assets:savings\n" +
			"\n"
		if err := os.WriteFile(filepath.Join(monthDir, "01.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		n, err := RenameAccountInJournalFiles(dir, "assets:checking", "assets:bank")
		if err != nil {
			t.Fatalf("RenameAccountInJournalFiles: %v", err)
		}
		if n != 2 {
			t.Errorf("expected 2 renames, got %d", n)
		}

		data, _ := os.ReadFile(filepath.Join(monthDir, "01.journal"))
		got := string(data)
		if strings.Contains(got, "assets:checking") {
			t.Errorf("old account name still present:\n%s", got)
		}
		if !strings.Contains(got, "assets:bank:joint") {
			t.Errorf("sub-account not renamed:\n%s", got)
		}
		if !strings.Contains(got, "assets:bank  ") {
			t.Errorf("exact account not renamed:\n%s", got)
		}
	})
}
