package ledger_test

import (
	"testing"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"

	"connectrpc.com/connect"
)

func TestSetStripeDailyImportEnabled(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 800, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{})

	if _, err := h.SetStripeDailyImportEnabled(t.Context(), connect.NewRequest(&floatv1.SetStripeDailyImportEnabledRequest{Enabled: true})); err != nil {
		t.Fatalf("SetStripeDailyImportEnabled(true): %v", err)
	}

	resp, err := h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetStripeConfig: %v", err)
	}
	if !resp.Msg.DailyImportEnabled {
		t.Error("DailyImportEnabled = false after enabling, want true")
	}

	if _, err := h.SetStripeDailyImportEnabled(t.Context(), connect.NewRequest(&floatv1.SetStripeDailyImportEnabledRequest{Enabled: false})); err != nil {
		t.Fatalf("SetStripeDailyImportEnabled(false): %v", err)
	}
	resp, err = h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetStripeConfig: %v", err)
	}
	if resp.Msg.DailyImportEnabled {
		t.Error("DailyImportEnabled = true after disabling, want false")
	}
}

func TestGetStripeConfigDailyImportFields(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 801, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{
		Stripe: config.StripeConfig{
			DailyImportEnabled: true,
			LastDailyImportAt:  "2026-05-15T12:00:00Z",
		},
	})

	resp, err := h.GetStripeConfig(t.Context(), connect.NewRequest(&floatv1.GetStripeConfigRequest{}))
	if err != nil {
		t.Fatalf("GetStripeConfig: %v", err)
	}
	if !resp.Msg.DailyImportEnabled {
		t.Error("DailyImportEnabled = false, want true (config has it enabled)")
	}
	if resp.Msg.LastDailyImportAt != "2026-05-15T12:00:00Z" {
		t.Errorf("LastDailyImportAt = %q, want %q", resp.Msg.LastDailyImportAt, "2026-05-15T12:00:00Z")
	}
}

// TestRunDailyStripeImport_NoLinkedAccounts verifies the auto-importer is a clean no-op
// when nothing is configured. The internal helper runs to completion without touching
// Stripe (no accounts to iterate) and reports zero imports / zero errors.
func TestRunDailyStripeImport_NoLinkedAccounts(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_xxx")
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 802, NumTxns: 1})
	h := mustHandlerWithConfig(t, dir, &config.Config{
		Stripe: config.StripeConfig{DailyImportEnabled: true},
	})

	imported, errs := serverledger.ExportedRunDailyStripeImport(h, t.Context())
	if imported != 0 {
		t.Errorf("imported = %d, want 0", imported)
	}
	if len(errs) != 0 {
		t.Errorf("errors = %v, want none", errs)
	}
}
