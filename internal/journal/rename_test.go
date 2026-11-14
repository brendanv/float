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

	t.Run("spaced_names", func(t *testing.T) {
		dir := t.TempDir()
		content := "; float: account declarations\n" +
			"account assets:bank checking  ; type: A\n" +
			"account assets:bank checking:joint\n" +
			"account assets:bank\n" +
			"account assets:bank savings\n"
		if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if err := RenameAccountDeclaration(dir, "assets:bank checking", "assets:chase"); err != nil {
			t.Fatalf("RenameAccountDeclaration: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "accounts.journal"))
		if err != nil {
			t.Fatal(err)
		}
		want := "; float: account declarations\n" +
			"account assets:chase  ; type: A\n" +
			"account assets:chase:joint\n" +
			"account assets:bank\n" +
			"account assets:bank savings\n"
		if string(data) != want {
			t.Errorf("accounts.journal =\n%s\nwant\n%s", data, want)
		}

		// Renaming the first word's account must not touch "assets:bank savings".
		if err := RenameAccountDeclaration(dir, "assets:bank", "assets:old bank"); err != nil {
			t.Fatalf("RenameAccountDeclaration: %v", err)
		}
		decls, err := ListAccountDeclarations(dir)
		if err != nil {
			t.Fatalf("ListAccountDeclarations: %v", err)
		}
		var names []string
		for _, d := range decls {
			names = append(names, d.Name)
		}
		wantNames := []string{"assets:chase", "assets:chase:joint", "assets:old bank", "assets:bank savings"}
		if strings.Join(names, "|") != strings.Join(wantNames, "|") {
			t.Errorf("declarations = %q, want %q", names, wantNames)
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
			// hledger accepts any indentation of one or more blanks for postings.
			name:    "single_space_indent_is_a_posting",
			line:    " assets:checking  100.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    " assets:bank  100.00 USD",
			changed: true,
		},
		{
			name:    "single_tab_indent_is_a_posting",
			line:    "\tassets:checking  100.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "\tassets:bank  100.00 USD",
			changed: true,
		},
		{
			name:    "unindented_line_not_touched",
			line:    "assets:checking  100.00 USD",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "assets:checking  100.00 USD",
			changed: false,
		},
		{
			name:    "blank_line_not_touched",
			line:    "    ",
			oldName: "assets:checking",
			newName: "assets:bank",
			want:    "    ",
			changed: false,
		},

		// Account names containing single spaces.
		{
			name:    "spaced_name_renamed",
			line:    "    assets:bank checking   $5",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    assets:chase   $5",
			changed: true,
		},
		{
			name:    "rename_to_spaced_name",
			line:    "    assets:bank  $5",
			oldName: "assets:bank",
			newName: "assets:bank checking",
			want:    "    assets:bank checking  $5",
			changed: true,
		},
		{
			name:    "spaced_name_sub_account_renamed",
			line:    "    assets:bank checking:joint  $5",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    assets:chase:joint  $5",
			changed: true,
		},
		{
			// "assets:bank checking" is a different account, not a sub-account.
			name:    "spaced_name_not_matched_by_first_word",
			line:    "    assets:bank checking   $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:bank checking   $5",
			changed: false,
		},
		{
			name:    "spaced_name_at_end_of_line",
			line:    "    assets:bank checking",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    assets:chase",
			changed: true,
		},
		{
			// A single trailing blank before end of line is not part of the name.
			name:    "trailing_single_space",
			line:    "    assets:bank ",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:chase ",
			changed: true,
		},

		// Status marks.
		{
			name:    "cleared_status_mark",
			line:    "    * assets:bank   $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    * assets:chase   $5",
			changed: true,
		},
		{
			name:    "pending_status_mark_no_amount",
			line:    "    ! assets:bank",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    ! assets:chase",
			changed: true,
		},
		{
			name:    "status_mark_without_space",
			line:    "    *assets:bank  $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    *assets:chase  $5",
			changed: true,
		},
		{
			name:    "status_mark_spaced_name",
			line:    "    * assets:bank checking  $5",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    * assets:chase  $5",
			changed: true,
		},

		// Virtual postings.
		{
			name:    "virtual_parens",
			line:    "    (assets:bank)  $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    (assets:chase)  $5",
			changed: true,
		},
		{
			name:    "balanced_virtual_brackets",
			line:    "    [assets:bank:savings]  $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    [assets:chase:savings]  $5",
			changed: true,
		},
		{
			name:    "virtual_with_status_and_spaced_name",
			line:    "    ! [assets:bank checking]",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    ! [assets:chase]",
			changed: true,
		},
		{
			name:    "virtual_non_matching",
			line:    "    (assets:bankx)  $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    (assets:bankx)  $5",
			changed: false,
		},

		// Separators between account and amount.
		{
			name:    "tab_separator",
			line:    "    assets:bank\t\t$5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:chase\t\t$5",
			changed: true,
		},
		{
			name:    "space_then_tab_separator",
			line:    "    assets:bank checking \t$5",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    assets:chase \t$5",
			changed: true,
		},
		{
			// Like hledger, a lone tab is a single space inside the name: this
			// line's account is "assets:bank $5", not "assets:bank".
			name:    "single_tab_is_part_of_name",
			line:    "    assets:bank\t$5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:bank\t$5",
			changed: false,
		},

		// Comments and other trailing text are preserved verbatim.
		{
			name:    "trailing_comment",
			line:    "    assets:bank  $5  ; foo",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:chase  $5  ; foo",
			changed: true,
		},
		{
			name:    "comment_without_amount",
			line:    "    assets:bank checking  ; foo: bar",
			oldName: "assets:bank checking",
			newName: "assets:chase",
			want:    "    assets:chase  ; foo: bar",
			changed: true,
		},
		{
			name:    "cost_and_assertion_preserved",
			line:    "    assets:bank    10 AAPL @ $5 = 20 AAPL",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:chase    10 AAPL @ $5 = 20 AAPL",
			changed: true,
		},
		{
			name:    "indented_comment_mentioning_account",
			line:    "    ; assets:bank  $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    ; assets:bank  $5",
			changed: false,
		},
		{
			name:    "indented_hash_comment",
			line:    "\t# assets:bank",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "\t# assets:bank",
			changed: false,
		},
		{
			name:    "non_matching_prefix",
			line:    "    assets:bankx  $5",
			oldName: "assets:bank",
			newName: "assets:chase",
			want:    "    assets:bankx  $5",
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
			Seed:     301,
			NumTxns:  3,
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
