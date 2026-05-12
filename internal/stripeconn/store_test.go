package stripeconn_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brendanv/float/internal/stripeconn"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := stripeconn.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Version != stripeconn.SchemaVersion {
		t.Errorf("Version: got %d, want %d", s.Version, stripeconn.SchemaVersion)
	}
	if len(s.Connections) != 0 {
		t.Errorf("Connections: got %d, want 0", len(s.Connections))
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := &stripeconn.Store{
		Connections: []stripeconn.Connection{
			{
				ID:                    "a1b2c3d4",
				StripeAccountID:       "fca_abc",
				DisplayName:           "Chase Checking",
				InstitutionName:       "Chase",
				Last4:                 "1234",
				AccountCategory:       stripeconn.CategoryCash,
				AccountSubcategory:    "checking",
				Currency:              "USD",
				HledgerAccount:        "assets:chase:checking",
				DefaultInflowAccount:  "income:unknown",
				DefaultOutflowAccount: "expenses:unknown",
				ImportedIDs:           []string{"tx_1", "tx_2"},
				CreatedAt:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	if err := stripeconn.Save(dir, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stripe", "connections.json")); err != nil {
		t.Fatalf("connections.json not written: %v", err)
	}

	out, err := stripeconn.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Version != stripeconn.SchemaVersion {
		t.Errorf("Version: got %d", out.Version)
	}
	if len(out.Connections) != 1 {
		t.Fatalf("Connections: got %d", len(out.Connections))
	}
	got := out.Connections[0]
	if got.ID != "a1b2c3d4" || got.HledgerAccount != "assets:chase:checking" {
		t.Errorf("connection mismatch: %+v", got)
	}
	if len(got.ImportedIDs) != 2 {
		t.Errorf("ImportedIDs: got %d, want 2", len(got.ImportedIDs))
	}
}

func TestUpsertAndDelete(t *testing.T) {
	s := &stripeconn.Store{}
	s.Upsert(stripeconn.Connection{ID: "aaa", DisplayName: "first"})
	s.Upsert(stripeconn.Connection{ID: "bbb", DisplayName: "second"})
	if len(s.Connections) != 2 {
		t.Fatalf("after inserts: got %d", len(s.Connections))
	}

	// Re-upserting "aaa" should replace, not append.
	s.Upsert(stripeconn.Connection{ID: "aaa", DisplayName: "first-updated"})
	if len(s.Connections) != 2 {
		t.Fatalf("after replace: got %d", len(s.Connections))
	}
	if got := s.Find("aaa"); got == nil || got.DisplayName != "first-updated" {
		t.Errorf("Find aaa: %+v", got)
	}

	if !s.Delete("aaa") {
		t.Fatal("Delete returned false")
	}
	if s.Find("aaa") != nil {
		t.Errorf("aaa still found after delete")
	}
	if s.Delete("missing") {
		t.Error("Delete on missing id returned true")
	}
}

func TestMarkImportedDedup(t *testing.T) {
	c := &stripeconn.Connection{}
	c.MarkImported("tx_1")
	c.MarkImported("tx_2")
	c.MarkImported("tx_1") // duplicate
	if len(c.ImportedIDs) != 2 {
		t.Errorf("ImportedIDs: got %d, want 2", len(c.ImportedIDs))
	}
	if !c.HasImported("tx_1") || !c.HasImported("tx_2") {
		t.Error("HasImported should report true for added ids")
	}
	if c.HasImported("tx_3") {
		t.Error("HasImported should report false for unseen id")
	}
}

func TestFindByStripeID(t *testing.T) {
	s := &stripeconn.Store{
		Connections: []stripeconn.Connection{
			{ID: "aaa", StripeAccountID: "fca_one"},
			{ID: "bbb", StripeAccountID: "fca_two"},
		},
	}
	got := s.FindByStripeID("fca_two")
	if got == nil || got.ID != "bbb" {
		t.Errorf("FindByStripeID: %+v", got)
	}
	if s.FindByStripeID("fca_nope") != nil {
		t.Error("FindByStripeID should return nil for missing id")
	}
}
