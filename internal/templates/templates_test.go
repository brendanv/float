package templates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanv/float/internal/templates"
)

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	ts, err := templates.Load(dir)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if len(ts) != 0 {
		t.Fatalf("Load: expected empty slice, got %d templates", len(ts))
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	input := []templates.Template{
		{
			ID:    "abc12345",
			Name:  "Mortgage Payment",
			Payee: "Bank of America",
			Note:  "Mortgage",
			Postings: []templates.TemplatePosting{
				{Account: "assets:checking", Commodity: "USD", DefaultQuantity: ""},
				{Account: "liabilities:mortgage:principal", Commodity: "USD", DefaultQuantity: "800.00"},
				{Account: "liabilities:mortgage:interest", Commodity: "USD", DefaultQuantity: "600.00"},
				{Account: "liabilities:mortgage:escrow", Commodity: "USD", DefaultQuantity: "100.00"},
			},
			Tags: map[string]string{"category": "housing"},
		},
	}

	if err := templates.Save(dir, input); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "templates.json")); err != nil {
		t.Fatalf("templates.json not created: %v", err)
	}

	got, err := templates.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Load: expected 1 template, got %d", len(got))
	}
	g := got[0]
	if g.ID != "abc12345" || g.Name != "Mortgage Payment" || g.Payee != "Bank of America" || g.Note != "Mortgage" {
		t.Errorf("template fields mismatch: %+v", g)
	}
	if len(g.Postings) != 4 {
		t.Fatalf("expected 4 postings, got %d", len(g.Postings))
	}
	if g.Postings[0].Account != "assets:checking" || g.Postings[0].DefaultQuantity != "" {
		t.Errorf("posting 0 mismatch: %+v", g.Postings[0])
	}
	if g.Postings[1].DefaultQuantity != "800.00" {
		t.Errorf("posting 1 default_quantity mismatch: %+v", g.Postings[1])
	}
	if g.Tags["category"] != "housing" {
		t.Errorf("tags mismatch: %+v", g.Tags)
	}
}

func TestSaveEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := templates.Save(dir, nil); err != nil {
		t.Fatalf("Save nil: %v", err)
	}
	got, err := templates.Load(dir)
	if err != nil {
		t.Fatalf("Load after save nil: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 templates, got %d", len(got))
	}
}
