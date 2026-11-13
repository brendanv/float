package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/txlock"
)

const (
	// maxStructuredImport caps one ImportStructuredTransactions request. Every
	// transaction is formatted through hledger under the write lock, so an
	// unbounded batch would hold the lock for a long time.
	maxStructuredImport = 5000

	defaultStructuredImportSource = "api"

	// importSourceMetaKey records the source label of a structured import
	// batch on each of its transactions, so ListImports can report it.
	importSourceMetaKey = hledger.HiddenMetaPrefix + "import-source"
)

func (h *Handler) ImportStructuredTransactions(ctx context.Context, req *connect.Request[floatv1.ImportStructuredTransactionsRequest]) (*connect.Response[floatv1.ImportStructuredTransactionsResponse], error) {
	msg := req.Msg
	if len(msg.Transactions) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("transactions must not be empty"))
	}
	if len(msg.Transactions) > maxStructuredImport {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("too many transactions: %d (max %d per request)", len(msg.Transactions), maxStructuredImport))
	}
	source, err := normalizeImportSource(msg.Source)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	var rulesList []rules.Rule
	if msg.ApplyRules {
		if rulesList, err = rules.Load(h.dataDir); err != nil {
			return nil, rpcErr(ctx, err, "load rules failed")
		}
	}

	inputs := make([]journal.TransactionInput, len(msg.Transactions))
	matchedRules := make([]string, len(msg.Transactions))
	for i, st := range msg.Transactions {
		in, ruleID, convErr := structuredToInput(st, rulesList)
		if convErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("transaction %d: %w", i, convErr))
		}
		inputs[i] = in
		matchedRules[i] = ruleID
	}

	// Parse through hledger: validates every transaction (balancing, amount
	// syntax) without touching the journal, and yields the canonical form
	// used for duplicate fingerprints and the preview.
	parsed, err := journal.ParseInputs(ctx, h.hl, inputs)
	if err != nil {
		var ie *journal.InputError
		if errors.As(err, &ie) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, rpcErr(ctx, err, "parse transactions failed")
	}

	fingerprints := make([]string, len(parsed))
	candidates := make([]*floatv1.ImportCandidate, len(parsed))
	for i, t := range parsed {
		fingerprints[i] = journal.TxnFingerprint(t)
		t.FID = "" // placeholder code from ParseInputs; the real FID is minted on write
		candidates[i] = &floatv1.ImportCandidate{
			Transaction:   toProtoTransaction(t),
			MatchedRuleId: matchedRules[i],
		}
	}

	if msg.DryRun {
		existing, err := cachedTransactions(ctx, h.cache, h.hl, nil)
		if err != nil {
			return nil, rpcErr(ctx, err, "fetch existing transactions failed")
		}
		fpSet := fingerprintSet(existing)
		for i := range candidates {
			candidates[i].IsDuplicate = fpSet[fingerprints[i]]
		}
		return connect.NewResponse(&floatv1.ImportStructuredTransactionsResponse{Candidates: candidates}), nil
	}

	batchID := source + "/" + time.Now().Format("2006-01-02") + "-" + journal.MintFID()
	var importedFIDs []string
	var skipped int32
	err = h.lock.Do(ctx, fmt.Sprintf("import %d structured transactions (batch %s)", len(inputs), batchID), func() error {
		// Fingerprint against the on-disk journal inside the lock rather than
		// the cache, which can be stale after external edits (see ImportTransactions).
		onDisk, fpErr := h.hl.Transactions(ctx)
		if fpErr != nil {
			return fmt.Errorf("re-read transactions for dedup: %w", fpErr)
		}
		fpSet := fingerprintSet(onDisk)
		for i, in := range inputs {
			dup := fpSet[fingerprints[i]]
			candidates[i].IsDuplicate = dup
			if dup && !msg.IncludeDuplicates {
				skipped++
				continue
			}
			in.FloatMeta = map[string]string{
				"float-import":      batchID,
				importSourceMetaKey: source,
			}
			fid, writeErr := journal.AppendTransaction(ctx, h.hl, h.dataDir, in)
			if writeErr != nil {
				return fmt.Errorf("write transaction %d: %w", i, writeErr)
			}
			importedFIDs = append(importedFIDs, fid)
		}
		if len(importedFIDs) == 0 {
			return txlock.ErrNoChanges
		}
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "import structured transactions failed")
	}

	resp := &floatv1.ImportStructuredTransactionsResponse{
		Candidates:            candidates,
		ImportedCount:         int32(len(importedFIDs)),
		SkippedDuplicateCount: skipped,
	}
	if len(importedFIDs) == 0 {
		return connect.NewResponse(resp), nil
	}
	resp.ImportBatchId = batchID

	txns, err := h.fetchByFIDs(ctx, importedFIDs)
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch imported transactions failed", "batch", batchID)
	}
	byFID := make(map[string]hledger.Transaction, len(txns))
	for _, t := range txns {
		byFID[t.FID] = t
	}
	for _, fid := range importedFIDs {
		if t, ok := byFID[fid]; ok {
			resp.Transactions = append(resp.Transactions, toProtoTransaction(t))
		}
	}
	return connect.NewResponse(resp), nil
}

// structuredToInput validates st and converts it to a TransactionInput,
// applying the first matching float rule when rulesList is non-empty. It
// returns the matched rule's ID ("" if none).
//
// Rules match the bare description (as CSV import matches the bank's
// description). An explicit payee, tag, or status on st wins over the rule's.
func structuredToInput(st *floatv1.StructuredTransaction, rulesList []rules.Rule) (journal.TransactionInput, string, error) {
	var zero journal.TransactionInput
	if st.Date == "" {
		return zero, "", errors.New("date is required")
	}
	date, err := time.Parse("2006-01-02", st.Date)
	if err != nil {
		return zero, "", fmt.Errorf("invalid date %q: must be YYYY-MM-DD", st.Date)
	}
	if strings.TrimSpace(st.Description) == "" {
		return zero, "", errors.New("description is required")
	}
	if strings.ContainsAny(st.Description+st.Payee, "\n\r") {
		return zero, "", errors.New("description and payee must be a single line")
	}
	if err := validateTxStatus(st.Status); err != nil {
		return zero, "", err
	}
	if len(st.Postings) < 2 {
		return zero, "", errors.New("at least 2 postings are required")
	}
	postings := make([]journal.PostingInput, len(st.Postings))
	for i, p := range st.Postings {
		if strings.TrimSpace(p.Account) == "" {
			return zero, "", fmt.Errorf("posting %d: account is required", i)
		}
		postings[i] = journal.PostingInput{
			Account:          p.Account,
			Commodity:        p.Commodity,
			Quantity:         p.Quantity,
			Comment:          p.Comment,
			Cost:             protoToJournalCost(p.Cost),
			CostBasis:        protoToJournalCostBasis(p.CostBasis),
			BalanceAssertion: protoToJournalAssertion(p.BalanceAssertion),
		}
	}
	var tags map[string]string
	if len(st.Tags) > 0 {
		tags = make(map[string]string, len(st.Tags))
		for k, v := range st.Tags {
			if strings.HasPrefix(k, hledger.HiddenMetaPrefix) {
				return zero, "", fmt.Errorf("tag %q: the %q prefix is reserved", k, hledger.HiddenMetaPrefix)
			}
			tags[k] = v
		}
	}

	payee, status := st.Payee, st.Status
	var ruleID string
	if r := rules.Match(rulesList, st.Description, sourceAccountFromInputs(postings)); r != nil {
		ruleID = r.ID
		if payee == "" {
			payee = r.Payee
		}
		if r.Account != "" && len(postings) == 2 {
			for j := range postings {
				if !isAssetOrLiabilityAccount(postings[j].Account) {
					postings[j].Account = r.Account
				}
			}
		}
		for k, v := range r.Tags {
			if tags == nil {
				tags = make(map[string]string, len(r.Tags))
			}
			if _, ok := tags[k]; !ok {
				tags[k] = v
			}
		}
		if r.AutoReviewed && status == "" {
			status = "Cleared"
		}
	}

	desc := st.Description
	if payee != "" {
		desc = payee + " | " + desc
	}
	return journal.TransactionInput{
		Date:        date,
		Description: desc,
		Comment:     st.Comment,
		Tags:        tags,
		Postings:    postings,
		Status:      status,
	}, ruleID, nil
}

// normalizeImportSource validates a structured-import source label, which
// becomes the batch ID prefix and ImportSummary.source.
func normalizeImportSource(s string) (string, error) {
	if s == "" {
		return defaultStructuredImportSource, nil
	}
	if len(s) > 32 {
		return "", fmt.Errorf("source %q: at most 32 characters", s)
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "", fmt.Errorf("source %q: only lowercase letters, digits and hyphens are allowed", s)
		}
	}
	if s == "csv" || s == "stripe" || strings.HasPrefix(s, "stripe-") {
		return "", fmt.Errorf("source %q is reserved", s)
	}
	return s, nil
}

func validateTxStatus(status string) error {
	switch status {
	case "", "Pending", "Cleared":
		return nil
	default:
		return fmt.Errorf("invalid status %q: must be \"\", \"Pending\", or \"Cleared\"", status)
	}
}

func sourceAccountFromInputs(postings []journal.PostingInput) string {
	for _, p := range postings {
		if isAssetOrLiabilityAccount(p.Account) {
			return p.Account
		}
	}
	return ""
}

func fingerprintSet(txns []hledger.Transaction) map[string]bool {
	set := make(map[string]bool, len(txns))
	for _, t := range txns {
		set[journal.TxnFingerprint(t)] = true
	}
	return set
}
