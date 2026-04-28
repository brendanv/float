package journal_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brendanv/float/internal/journal"
)

func TestEnsureCommodityDirective(t *testing.T) {
	t.Run("inserts directive before includes", func(t *testing.T) {
		dir := t.TempDir()
		main := "include accounts.journal\ninclude 2026/01.journal\n"
		if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
			t.Fatal(err)
		}

		if err := journal.EnsureCommodityDirective(dir, "USD"); err != nil {
			t.Fatal(err)
		}

		data, _ := os.ReadFile(filepath.Join(dir, "main.journal"))
		content := string(data)
		if !strings.Contains(content, "commodity 1,000.00 USD") {
			t.Errorf("directive not found:\n%s", content)
		}
		// Directive should appear before first include.
		idx := strings.Index(content, "commodity 1,000.00 USD")
		incIdx := strings.Index(content, "include ")
		if idx > incIdx {
			t.Errorf("directive should appear before includes:\n%s", content)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		dir := t.TempDir()
		main := "commodity 1,000.00 USD\ninclude accounts.journal\n"
		if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
			t.Fatal(err)
		}

		if err := journal.EnsureCommodityDirective(dir, "USD"); err != nil {
			t.Fatal(err)
		}

		data, _ := os.ReadFile(filepath.Join(dir, "main.journal"))
		if strings.Count(string(data), "commodity 1,000.00 USD") != 1 {
			t.Errorf("directive duplicated:\n%s", data)
		}
	})

	t.Run("creates in empty file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.journal"), nil, 0644); err != nil {
			t.Fatal(err)
		}

		if err := journal.EnsureCommodityDirective(dir, "USD"); err != nil {
			t.Fatal(err)
		}

		data, _ := os.ReadFile(filepath.Join(dir, "main.journal"))
		if !strings.Contains(string(data), "commodity 1,000.00 USD") {
			t.Errorf("directive not found:\n%s", data)
		}
	})
}
