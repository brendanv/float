package ledger_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
	"github.com/brendanv/float/internal/txlock"
)

func TestImportTransactionsHandler(t *testing.T) {
	t.Run("rule_with_tags_applied_without_panic", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 300, NumTxns: 1, WithFIDs: true})

		hledgerRules := "skip 1\nfields date, description, amount\naccount1 assets:checking\n\nif AMAZON\n  account2 expenses:shopping\n"
		if err := os.WriteFile(filepath.Join(dir, "bank.rules"), []byte(hledgerRules), 0o644); err != nil {
			t.Fatalf("write bank.rules: %v", err)
		}

		floatRulesJSON := `[{"id":"abcd1234","pattern":"AMAZON","payee":"","account":"","tags":{"source":"amazon"},"priority":0,"auto_reviewed":false}]`
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(floatRulesJSON), 0o644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		lock := txlock.New(dir, c)
		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "test-bank", RulesFile: "bank.rules"},
			},
		}
		h := serverledger.NewHandler(c, lock, dir, "", nil, nil, cfg, nil)

		csvData := []byte("date,description,amount\n2026-01-15,AMAZON MARKETPLACE,-45.00\n")
		result, err := importTransactions(t, h, &floatv1.ImportTransactionsRequest{
			CsvData:          csvData,
			ProfileName:      "test-bank",
			CandidateIndices: []int32{0},
		})
		if err != nil {
			t.Fatalf("ImportTransactions: %v", err)
		}
		if result.ImportedCount != 1 {
			t.Errorf("ImportedCount = %d, want 1", result.ImportedCount)
		}
		if len(result.Transactions) != 1 {
			t.Fatalf("expected 1 transaction, got %d", len(result.Transactions))
		}
		if result.Transactions[0].Tags["source"] != "amazon" {
			t.Errorf("Tags[source] = %q, want %q", result.Transactions[0].Tags["source"], "amazon")
		}
	})

	t.Run("reimport_detected_as_duplicate_when_rule_modifies_description", func(t *testing.T) {
		// Regression test: fingerprint must be computed after rules are applied so
		// that re-importing the same CSV detects the transaction as a duplicate.
		// Previously, IsDuplicate was set before the payee/account rule was applied,
		// so the stored fingerprint (post-rule description) never matched the
		// candidate fingerprint (pre-rule description).
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 302, NumTxns: 0, WithFIDs: true})

		hledgerRules := "skip 1\nfields date, description, amount\naccount1 assets:checking\naccount2 expenses:misc\n"
		if err := os.WriteFile(filepath.Join(dir, "bank.rules"), []byte(hledgerRules), 0o644); err != nil {
			t.Fatalf("write bank.rules: %v", err)
		}

		floatRulesJSON := `[{"id":"rule1","pattern":"COFFEE","payee":"Local Coffee","account":"expenses:food:coffee","tags":{},"priority":0,"auto_reviewed":false}]`
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(floatRulesJSON), 0o644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		lock := txlock.New(dir, c)
		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "test-bank", RulesFile: "bank.rules"},
			},
		}
		h := serverledger.NewHandler(c, lock, dir, "", nil, nil, cfg, nil)

		csvData := []byte("date,description,amount\n2026-03-10,COFFEE SHOP,-5.50\n")

		// First import: transaction is new.
		_, err = importTransactions(t, h, &floatv1.ImportTransactionsRequest{
			CsvData:          csvData,
			ProfileName:      "test-bank",
			CandidateIndices: []int32{0},
		})
		if err != nil {
			t.Fatalf("first ImportTransactions: %v", err)
		}

		// Second preview of the same CSV: should be detected as a duplicate.
		mux := http.NewServeMux()
		path, handler := floatv1connect.NewLedgerServiceHandler(h)
		mux.Handle(path, handler)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)

		previewResp, err := client.PreviewImport(t.Context(), connect.NewRequest(&floatv1.PreviewImportRequest{
			CsvData:     csvData,
			ProfileName: "test-bank",
		}))
		if err != nil {
			t.Fatalf("PreviewImport: %v", err)
		}
		if len(previewResp.Msg.Candidates) != 1 {
			t.Fatalf("expected 1 candidate, got %d", len(previewResp.Msg.Candidates))
		}
		if !previewResp.Msg.Candidates[0].IsDuplicate {
			t.Errorf("IsDuplicate = false, want true — previously imported transaction not detected as duplicate")
		}
	})

	t.Run("reimport_skipped_in_lock_when_rule_modifies_payee_and_account", func(t *testing.T) {
		// Regression test: the in-lock dedup in ImportTransactions must
		// fingerprint the post-rule candidate (the form written to disk), not
		// the raw CSV row, or re-importing a rule-matched row writes a duplicate.
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 303, NumTxns: 0, WithFIDs: true})

		hledgerRules := "skip 1\nfields date, description, amount\naccount1 assets:checking\naccount2 expenses:misc\n"
		if err := os.WriteFile(filepath.Join(dir, "bank.rules"), []byte(hledgerRules), 0o644); err != nil {
			t.Fatalf("write bank.rules: %v", err)
		}

		floatRulesJSON := `[{"id":"rule1","pattern":"AMAZON","payee":"Amazon","account":"expenses:shopping","tags":{},"priority":0,"auto_reviewed":false}]`
		if err := os.WriteFile(filepath.Join(dir, "rules.json"), []byte(floatRulesJSON), 0o644); err != nil {
			t.Fatalf("write rules.json: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		lock := txlock.New(dir, c)
		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "test-bank", RulesFile: "bank.rules"},
			},
		}
		h := serverledger.NewHandler(c, lock, dir, "", nil, nil, cfg, nil)

		req := &floatv1.ImportTransactionsRequest{
			CsvData:          []byte("date,description,amount\n2026-01-15,AMAZON MARKETPLACE,-45.00\n"),
			ProfileName:      "test-bank",
			CandidateIndices: []int32{0},
		}

		first, err := importTransactions(t, h, req)
		if err != nil {
			t.Fatalf("first ImportTransactions: %v", err)
		}
		if first.ImportedCount != 1 || first.SkippedDuplicateCount != 0 {
			t.Fatalf("first import: imported=%d skipped=%d, want 1/0", first.ImportedCount, first.SkippedDuplicateCount)
		}

		second, err := importTransactions(t, h, req)
		if err != nil {
			t.Fatalf("second ImportTransactions: %v", err)
		}
		if second.ImportedCount != 0 {
			t.Errorf("second import ImportedCount = %d, want 0", second.ImportedCount)
		}
		if second.SkippedDuplicateCount != 1 {
			t.Errorf("second import SkippedDuplicateCount = %d, want 1", second.SkippedDuplicateCount)
		}
	})

	t.Run("import_batch_id_populated_on_imported_transactions", func(t *testing.T) {
		dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 301, NumTxns: 1, WithFIDs: true})

		hledgerRules := "skip 1\nfields date, description, amount\naccount1 assets:checking\naccount2 expenses:misc\n"
		if err := os.WriteFile(filepath.Join(dir, "bank.rules"), []byte(hledgerRules), 0o644); err != nil {
			t.Fatalf("write bank.rules: %v", err)
		}

		c, err := hledger.New("hledger", dir+"/main.journal")
		if err != nil {
			t.Skipf("hledger unavailable: %v", err)
		}
		lock := txlock.New(dir, c)
		cfg := &config.Config{
			BankProfiles: []config.BankProfile{
				{Name: "test-bank", RulesFile: "bank.rules"},
			},
		}
		h := serverledger.NewHandler(c, lock, dir, "", nil, nil, cfg, nil)

		csvData := []byte("date,description,amount\n2026-01-15,COFFEE SHOP,-4.50\n2026-01-16,GROCERY STORE,-30.00\n")
		result, err := importTransactions(t, h, &floatv1.ImportTransactionsRequest{
			CsvData:          csvData,
			ProfileName:      "test-bank",
			CandidateIndices: []int32{0, 1},
		})
		if err != nil {
			t.Fatalf("ImportTransactions: %v", err)
		}
		batchID := result.ImportBatchId
		if batchID == "" {
			t.Fatal("response ImportBatchId is empty")
		}
		if len(result.Transactions) != 2 {
			t.Fatalf("expected 2 imported transactions, got %d", len(result.Transactions))
		}
		for i, tx := range result.Transactions {
			if tx.ImportBatchId == nil {
				t.Errorf("imported[%d].ImportBatchId = nil, want %q", i, batchID)
				continue
			}
			if *tx.ImportBatchId != batchID {
				t.Errorf("imported[%d].ImportBatchId = %q, want %q", i, *tx.ImportBatchId, batchID)
			}
		}

		// Round-trip verification: re-fetch via ListTransactions to ensure the
		// field survives parsing from hledger output.
		listResp, err := h.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}

		var withBatch, withoutBatch int
		for _, tx := range listResp.Msg.Transactions {
			if tx.ImportBatchId == nil {
				withoutBatch++
				continue
			}
			if *tx.ImportBatchId != batchID {
				t.Errorf("unexpected ImportBatchId %q on txn %s", *tx.ImportBatchId, tx.Fid)
				continue
			}
			withBatch++
		}
		if withBatch != 2 {
			t.Errorf("transactions with batch id = %d, want 2", withBatch)
		}
		if withoutBatch != 1 {
			t.Errorf("transactions without batch id = %d, want 1 (the seeded txn)", withoutBatch)
		}
	})
}
