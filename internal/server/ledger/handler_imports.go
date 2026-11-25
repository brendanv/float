package ledger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
)

func (h *Handler) PreviewImport(ctx context.Context, req *connect.Request[floatv1.PreviewImportRequest]) (*connect.Response[floatv1.PreviewImportResponse], error) {
	logger := slogctx.FromContext(ctx)
	if len(req.Msg.CsvData) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("csv_data is required"))
	}
	if req.Msg.ProfileName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("profile_name is required"))
	}

	// Find bank profile.
	profile, err := h.bankProfile(req.Msg.ProfileName)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	rulesFile := filepath.Join(h.dataDir, profile.RulesFile)

	// Write CSV to temp file.
	tmp, err := os.CreateTemp("", "float-import-*.csv")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create temp file: %w", err))
	}
	defer func() { _ = os.Remove(tmp.Name()) }() //nolint:forbidigo // temp file outside the data dir
	if _, err := tmp.Write(req.Msg.CsvData); err != nil {
		_ = tmp.Close()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	_ = tmp.Close()

	// Parse CSV with hledger.
	candidates, err := h.hl.PrintCSV(ctx, tmp.Name(), rulesFile)
	if err != nil {
		logger.ErrorContext(ctx, "hledger PrintCSV failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("parse CSV: %w", err))
	}

	// Build fingerprint set from existing transactions.
	existing, err := cachedTransactions(ctx, h.cache, h.hl, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch existing transactions failed")
	}
	fpSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		fpSet[journal.TxnFingerprint(t)] = true
	}

	// Load float rules for second-pass categorization.
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "load rules failed")
	}

	out := make([]*floatv1.ImportCandidate, len(candidates))
	for i, c := range candidates {
		candidate := &floatv1.ImportCandidate{}
		if !profile.SkipRules {
			if r := rules.Match(rulesList, c.Description, rules.SourceAccount(c.Postings)); r != nil {
				candidate.MatchedRuleId = r.ID
				// Apply rule transformations so the preview reflects what will actually be imported.
				c = rules.ApplyToCandidate(c, r)
			}
		}
		// Fingerprint after rules so it matches the form written to disk by ImportTransactions.
		candidate.IsDuplicate = fpSet[journal.TxnFingerprint(c)]
		candidate.Transaction = toProtoTransaction(c)
		out[i] = candidate
	}
	return connect.NewResponse(&floatv1.PreviewImportResponse{Candidates: out}), nil
}

func (h *Handler) ImportTransactions(ctx context.Context, req *connect.Request[floatv1.ImportTransactionsRequest], stream *connect.ServerStream[floatv1.ImportTransactionsResponse]) error {
	logger := slogctx.FromContext(ctx)
	if len(req.Msg.CsvData) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("csv_data is required"))
	}
	if req.Msg.ProfileName == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("profile_name is required"))
	}
	if len(req.Msg.CandidateIndices) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("candidate_indices must not be empty"))
	}

	profile, err := h.bankProfile(req.Msg.ProfileName)
	if err != nil {
		return connect.NewError(connect.CodeNotFound, err)
	}
	rulesFile := filepath.Join(h.dataDir, profile.RulesFile)

	// Write CSV to temp file.
	tmp, err := os.CreateTemp("", "float-import-*.csv")
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("create temp file: %w", err))
	}
	defer func() { _ = os.Remove(tmp.Name()) }() //nolint:forbidigo // temp file outside the data dir
	if _, err := tmp.Write(req.Msg.CsvData); err != nil {
		_ = tmp.Close()
		return connect.NewError(connect.CodeInternal, fmt.Errorf("write temp file: %w", err))
	}
	_ = tmp.Close()

	candidates, err := h.hl.PrintCSV(ctx, tmp.Name(), rulesFile)
	if err != nil {
		logger.ErrorContext(ctx, "hledger PrintCSV failed", "error", err)
		return connect.NewError(connect.CodeInternal, fmt.Errorf("parse CSV: %w", err))
	}

	// Load rules for categorization during import.
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return rpcErr(ctx, err, "load rules failed")
	}

	// Build selected indices set.
	selectedSet := make(map[int32]bool, len(req.Msg.CandidateIndices))
	for _, idx := range req.Msg.CandidateIndices {
		selectedSet[idx] = true
	}

	importBatchID := profileToSlug(req.Msg.ProfileName) + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	total := int32(len(req.Msg.CandidateIndices))
	uploadFilePath := filepath.Join(h.dataDir, "uploads", filepath.FromSlash(importBatchID+".csv"))

	var importedFIDs []string
	var skippedDuplicates int32
	err = h.lock.DoWith(ctx, fmt.Sprintf("import %d transactions (batch %s)", len(req.Msg.CandidateIndices), importBatchID), []string{uploadFilePath}, func() error {
		// Re-fingerprint against the on-disk journal inside the lock: the
		// preview's duplicate flags came from the cache, which is only
		// invalidated by floatd's own writes and can be stale if the journal
		// was edited externally (hand edit, floatctl, another tool) since the
		// preview was fetched. h.hl.Transactions bypasses the cache
		// deliberately, per the fetchByFIDs boundary comment above.
		onDisk, fpErr := h.hl.Transactions(ctx)
		if fpErr != nil {
			return fmt.Errorf("re-read transactions for dedup: %w", fpErr)
		}
		fpSet := make(map[string]bool, len(onDisk))
		for _, t := range onDisk {
			fpSet[journal.TxnFingerprint(t)] = true
		}

		for i, c := range candidates {
			if !selectedSet[int32(i)] {
				continue
			}
			// Match rules on the raw CSV description, then fingerprint and write
			// the post-rule form so dedup matches what earlier imports wrote.
			var rule *rules.Rule
			if !profile.SkipRules {
				rule = rules.Match(rulesList, c.Description, rules.SourceAccount(c.Postings))
			}
			c = rules.ApplyToCandidate(c, rule)
			if fpSet[journal.TxnFingerprint(c)] {
				skippedDuplicates++
				continue
			}
			txInput, convErr := journal.HledgerTxnToInput(c)
			if convErr != nil {
				return fmt.Errorf("convert transaction %d: %w", i, convErr)
			}

			// Stamp every transaction with the import batch ID as hidden metadata.
			if txInput.FloatMeta == nil {
				txInput.FloatMeta = make(map[string]string)
			}
			txInput.FloatMeta["float-import"] = importBatchID

			// Apply the remaining (non-fingerprinted) rule effects.
			rules.ApplyTagsAndStatus(&txInput, rule)

			fid, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, txInput)
			if writeErr != nil {
				return fmt.Errorf("write transaction %d: %w", i, writeErr)
			}
			importedFIDs = append(importedFIDs, fid)

			if sendErr := stream.Send(&floatv1.ImportTransactionsResponse{
				Payload: &floatv1.ImportTransactionsResponse_Progress{
					Progress: &floatv1.ImportProgress{
						Imported: int32(len(importedFIDs)),
						Total:    total,
					},
				},
			}); sendErr != nil {
				return sendErr
			}
		}

		// Save a copy of the uploaded CSV inside the lock so it is included in the git commit.
		// importBatchID may contain a "/" (profile-slug/date-fid), so we create the subdirectory.
		if mkErr := os.MkdirAll(filepath.Dir(uploadFilePath), 0o755); mkErr != nil {
			logger.ErrorContext(ctx, "create uploads dir failed", "error", mkErr)
		} else if wErr := os.WriteFile(uploadFilePath, req.Msg.CsvData, 0o644); wErr != nil { //nolint:forbidigo // inside txlock.DoWith
			logger.ErrorContext(ctx, "save uploaded CSV failed", "error", wErr)
		}
		return nil
	})
	if err != nil {
		return rpcErr(ctx, err, "import transactions failed")
	}

	txnProtos := h.importedTransactionProtos(ctx, importedFIDs)

	return stream.Send(&floatv1.ImportTransactionsResponse{
		Payload: &floatv1.ImportTransactionsResponse_Result{
			Result: &floatv1.ImportTransactionsResult{
				ImportedCount:         int32(len(importedFIDs)),
				Transactions:          txnProtos,
				ImportBatchId:         importBatchID,
				SkippedDuplicateCount: skippedDuplicates,
			},
		},
	})
}

func (h *Handler) GetImportedTransactions(ctx context.Context, req *connect.Request[floatv1.GetImportedTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	if req.Msg.ImportBatchId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("import_batch_id is required"))
	}
	query := []string{"tag:float-import=" + req.Msg.ImportBatchId}
	txns, err := filteredTransactions(ctx, h.cache, h.hl, query)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}
	txns, total, hasNext := paginate(txns, req.Msg.Offset, req.Msg.Limit)
	proto := make([]*floatv1.Transaction, len(txns))
	for i, t := range txns {
		proto[i] = toProtoTransaction(t)
	}
	return connect.NewResponse(&floatv1.ListTransactionsResponse{Transactions: proto, Total: total, HasNext: hasNext}), nil
}

func (h *Handler) ListImports(ctx context.Context, _ *connect.Request[floatv1.ListImportsRequest]) (*connect.Response[floatv1.ListImportsResponse], error) {
	txns, err := filteredTransactions(ctx, h.cache, h.hl, []string{"tag:float-import"})
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}

	// Group transactions by import batch ID, preserving newest-first order.
	type batchEntry struct {
		batchID string
		date    string
		source  string // structured-import source label; "" for CSV/Stripe batches
		count   int32
	}
	seen := make(map[string]int) // batchID -> index in batches
	var batches []batchEntry
	for _, t := range txns {
		batchID := t.FloatMeta["float-import"]
		if batchID == "" {
			continue
		}
		if idx, ok := seen[batchID]; ok {
			batches[idx].count++
		} else {
			// Extract date from the batch ID. New format is "profile-slug/YYYY-MM-DD-fid";
			// legacy format is "YYYY-MM-DD-fid". In both cases the date is the first 10
			// characters of the last path segment.
			datePart := batchID
			if slashIdx := strings.LastIndex(batchID, "/"); slashIdx >= 0 {
				datePart = batchID[slashIdx+1:]
			}
			date := ""
			if len(datePart) >= 10 {
				date = datePart[:10]
			}
			seen[batchID] = len(batches)
			batches = append(batches, batchEntry{batchID: batchID, date: date, source: t.FloatMeta[importSourceMetaKey], count: 1})
		}
	}

	// Sort descending by date, then by batch ID for stable ordering within the same date.
	sort.Slice(batches, func(i, j int) bool {
		if batches[i].date != batches[j].date {
			return batches[i].date > batches[j].date
		}
		return batches[i].batchID > batches[j].batchID
	})

	out := make([]*floatv1.ImportSummary, len(batches))
	for i, b := range batches {
		source := b.source
		switch {
		case source != "":
		case strings.HasPrefix(b.batchID, "stripe-"):
			source = "stripe"
		default:
			source = "csv"
		}
		out[i] = &floatv1.ImportSummary{
			ImportBatchId:    b.batchID,
			Date:             b.date,
			TransactionCount: b.count,
			Source:           source,
		}
	}
	return connect.NewResponse(&floatv1.ListImportsResponse{Imports: out}), nil
}

func (h *Handler) GetImportFile(ctx context.Context, req *connect.Request[floatv1.GetImportFileRequest]) (*connect.Response[floatv1.GetImportFileResponse], error) {
	if req.Msg.ImportBatchId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("import_batch_id is required"))
	}
	uploadsDir := filepath.Clean(filepath.Join(h.dataDir, "uploads"))
	filePath := filepath.Join(uploadsDir, filepath.FromSlash(req.Msg.ImportBatchId+".csv"))
	if !strings.HasPrefix(filepath.Clean(filePath), uploadsDir+string(filepath.Separator)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid import_batch_id"))
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("import file not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read import file: %w", err))
	}
	return connect.NewResponse(&floatv1.GetImportFileResponse{
		CsvContent: data,
		Filename:   req.Msg.ImportBatchId + ".csv",
	}), nil
}
