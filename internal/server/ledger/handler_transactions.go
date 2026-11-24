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
	"github.com/brendanv/float/internal/slogctx"
)

// fetchByFIDs returns the transactions matching fids, selected in Go from the
// full cached transaction list rather than a per-FID or per-batch hledger
// query. Write RPCs use this for their post-write re-read: it costs the same
// full parse as a "code:" query would, but populates the transactions:(nil)
// cache entry for the new generation as a side effect, and shares that load
// with any concurrent request via cache/singleflight.
//
// Do not use this for in-lock re-checks (e.g. Stripe dedup) — the journal is
// mid-mutation there and the cache generation is stale until the write
// commits; those call sites must keep using h.hl.Transactions directly.
func (h *Handler) fetchByFIDs(ctx context.Context, fids []string) ([]hledger.Transaction, error) {
	all, err := cachedTransactions(ctx, h.cache, h.hl, nil)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(fids))
	for _, fid := range fids {
		want[fid] = true
	}
	out := make([]hledger.Transaction, 0, len(fids))
	for _, t := range all {
		if want[t.FID] {
			out = append(out, t)
		}
	}
	return out, nil
}

// importedTransactionProtos re-reads the transactions an import just wrote
// (via fetchByFIDs, so a single shared full fetch rather than one hledger
// process per FID) and returns them in fids order. It is best-effort: FIDs
// that are not found are skipped, and a fetch error is logged and yields no
// transactions, since the write itself has already committed.
func (h *Handler) importedTransactionProtos(ctx context.Context, fids []string) []*floatv1.Transaction {
	if len(fids) == 0 {
		return nil
	}
	txns, err := h.fetchByFIDs(ctx, fids)
	if err != nil {
		slogctx.FromContext(ctx).WarnContext(ctx, "re-read imported transactions failed", "count", len(fids), "error", err)
		return nil
	}
	byFID := make(map[string]hledger.Transaction, len(txns))
	for _, t := range txns {
		byFID[t.FID] = t
	}
	out := make([]*floatv1.Transaction, 0, len(fids))
	for _, fid := range fids {
		if t, ok := byFID[fid]; ok {
			out = append(out, toProtoTransaction(t))
		}
	}
	return out
}

func paginate[T any](items []T, offset, limit int32) ([]T, int32, bool) {
	total := int32(len(items))
	if offset > 0 {
		if int(offset) >= len(items) {
			items = nil
		} else {
			items = items[offset:]
		}
	}
	hasNext := false
	if limit > 0 && int(limit) < len(items) {
		items = items[:limit]
		hasNext = true
	}
	return items, total, hasNext
}

func (h *Handler) ListTransactions(ctx context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
	txns, err := filteredTransactions(ctx, h.cache, h.hl, req.Msg.Query)
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

func (h *Handler) DeleteTransaction(ctx context.Context, req *connect.Request[floatv1.DeleteTransactionRequest]) (*connect.Response[floatv1.DeleteTransactionResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("delete transaction %s", fid), func() error {
		return journal.DeleteTransaction(ctx, h.hl, h.dataDir, fid)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete transaction failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.DeleteTransactionResponse{}), nil
}

func (h *Handler) ModifyTags(ctx context.Context, req *connect.Request[floatv1.ModifyTagsRequest]) (*connect.Response[floatv1.ModifyTagsResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("modify tags on transaction %s", fid), func() error {
		return journal.ModifyTags(ctx, h.hl, h.dataDir, fid, req.Msg.Tags)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "modify tags failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.ModifyTagsResponse{}), nil
}

func (h *Handler) UpdateTransactionDate(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionDateRequest]) (*connect.Response[floatv1.UpdateTransactionDateResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	if req.Msg.NewDate == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new_date is required"))
	}
	var updated hledger.Transaction
	err := h.lock.Do(ctx, fmt.Sprintf("update date on transaction %s", fid), func() error {
		var e error
		updated, e = journal.UpdateTransactionDate(ctx, h.hl, h.dataDir, fid, req.Msg.NewDate)
		return e
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if errors.Is(err, journal.ErrInvalidDate) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, rpcErr(ctx, err, "update transaction date failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.UpdateTransactionDateResponse{
		Transaction: toProtoTransaction(updated),
	}), nil
}

func (h *Handler) AddTransaction(ctx context.Context, req *connect.Request[floatv1.AddTransactionRequest]) (*connect.Response[floatv1.AddTransactionResponse], error) {
	if req.Msg.Description == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("description is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for i, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("posting %d: account is required", i))
		}
	}
	// Unset status keeps the historical default of "Cleared" (the web UI's
	// manual entry); automated clients send "" to leave it for review.
	status := "Cleared"
	if req.Msg.Status != nil {
		if err := validateTxStatus(*req.Msg.Status); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		status = *req.Msg.Status
	}

	var date time.Time
	if req.Msg.Date == "" {
		date = time.Now().UTC().Truncate(24 * time.Hour)
	} else {
		var err error
		date, err = time.Parse("2006-01-02", req.Msg.Date)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid date %q: must be YYYY-MM-DD", req.Msg.Date))
		}
	}

	postings := make([]journal.PostingInput, len(req.Msg.Postings))
	for i, p := range req.Msg.Postings {
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
	desc := req.Msg.Description
	if req.Msg.Payee != "" {
		desc = req.Msg.Payee + " | " + desc
	}
	tx := journal.TransactionInput{
		Date:        date,
		Description: desc,
		Comment:     req.Msg.Comment,
		Tags:        req.Msg.Tags,
		Postings:    postings,
		Status:      status,
	}

	var fid string
	err := h.lock.Do(ctx, fmt.Sprintf("add transaction: %s", desc), func() error {
		var e error
		fid, e = journal.AppendTransaction(ctx, h.hl, h.dataDir, tx)
		return e
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add transaction failed")
	}

	txns, err := h.fetchByFIDs(ctx, []string{fid})
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch new transaction failed", "fid", fid)
	}
	if len(txns) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after add", fid))
	}
	return connect.NewResponse(&floatv1.AddTransactionResponse{
		Transaction: toProtoTransaction(txns[0]),
	}), nil
}

func (h *Handler) UpdateTransaction(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionRequest]) (*connect.Response[floatv1.UpdateTransactionResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	if req.Msg.Description == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("description is required"))
	}
	if len(req.Msg.Postings) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least 2 postings are required"))
	}
	for i, p := range req.Msg.Postings {
		if p.Account == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("posting %d: account is required", i))
		}
	}

	postings := make([]journal.PostingInput, len(req.Msg.Postings))
	for i, p := range req.Msg.Postings {
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

	desc := req.Msg.Description
	if req.Msg.Payee != "" {
		desc = req.Msg.Payee + " | " + desc
	}

	// Proto3 can't tell an unset comment/map from an empty one, so empty
	// values keep the existing comment/tags unless replace_* asks to clear.
	var comment *string
	if req.Msg.Comment != "" || req.Msg.ReplaceComment {
		comment = &req.Msg.Comment
	}
	tags := req.Msg.Tags
	if len(tags) == 0 {
		tags = nil
		if req.Msg.ReplaceTags {
			tags = map[string]string{}
		}
	}

	var updated hledger.Transaction
	err := h.lock.Do(ctx, fmt.Sprintf("update transaction %s", fid), func() error {
		var e error
		updated, e = journal.UpdateTransaction(ctx, h.hl, h.dataDir, fid, desc, req.Msg.Date, comment, tags, postings, req.Msg.Status)
		return e
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if errors.Is(err, journal.ErrInvalidDate) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, rpcErr(ctx, err, "update transaction failed", "fid", fid)
	}
	return connect.NewResponse(&floatv1.UpdateTransactionResponse{
		Transaction: toProtoTransaction(updated),
	}), nil
}

func (h *Handler) UpdateTransactionStatus(ctx context.Context, req *connect.Request[floatv1.UpdateTransactionStatusRequest]) (*connect.Response[floatv1.UpdateTransactionStatusResponse], error) {
	fid := req.Msg.Fid
	if fid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fid is required"))
	}
	switch req.Msg.Status {
	case "", "Pending", "Cleared":
		// valid
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid status %q: must be \"\", \"Pending\", or \"Cleared\"", req.Msg.Status))
	}
	var statusMsg string
	switch req.Msg.Status {
	case "Cleared":
		statusMsg = "mark transaction " + fid + " as cleared"
	case "Pending":
		statusMsg = "mark transaction " + fid + " as pending"
	case "":
		statusMsg = "unmark transaction " + fid
	}
	err := h.lock.Do(ctx, statusMsg, func() error {
		return journal.UpdateTransactionStatus(ctx, h.hl, h.dataDir, fid, req.Msg.Status)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update transaction status failed", "fid", fid)
	}
	txns, err := h.fetchByFIDs(ctx, []string{fid})
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch transaction after status update failed", "fid", fid)
	}
	if len(txns) == 0 {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after status update", fid))
	}
	return connect.NewResponse(&floatv1.UpdateTransactionStatusResponse{
		Transaction: toProtoTransaction(txns[0]),
	}), nil
}

func (h *Handler) BulkEditTransactions(ctx context.Context, req *connect.Request[floatv1.BulkEditTransactionsRequest]) (*connect.Response[floatv1.BulkEditTransactionsResponse], error) {

	if len(req.Msg.Fids) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("fids must not be empty"))
	}
	if len(req.Msg.Operations) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("operations must not be empty"))
	}
	deleteOp := false
	for i, op := range req.Msg.Operations {
		switch v := op.Operation.(type) {
		case *floatv1.BulkEditOperation_MarkReviewed:
			// no additional validation needed
		case *floatv1.BulkEditOperation_AddTag:
			if v.AddTag.Key == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: add_tag key must not be empty", i))
			}
			if strings.HasPrefix(v.AddTag.Key, hledger.HiddenMetaPrefix) {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: add_tag key must not use reserved prefix %q", i, hledger.HiddenMetaPrefix))
			}
		case *floatv1.BulkEditOperation_RemoveTag:
			if v.RemoveTag.Key == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: remove_tag key must not be empty", i))
			}
		case *floatv1.BulkEditOperation_SetPayee:
			if v.SetPayee.Payee == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: set_payee payee must not be empty", i))
			}
		case *floatv1.BulkEditOperation_ClearPayee:
			// no additional validation needed
		case *floatv1.BulkEditOperation_UpdateUnknownAccount:
			if v.UpdateUnknownAccount.Account == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: update_unknown_account account must not be empty", i))
			}
		case *floatv1.BulkEditOperation_Delete:
			deleteOp = true
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("operation %d: unrecognized or missing operation type", i))
		}
	}
	if deleteOp {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("use BulkDeleteTransactions for bulk delete operations"))
	}

	type pendingEdit struct {
		input journal.TransactionInput
		src   journal.SourceLocation
		fid   string
	}

	err := h.lock.Do(ctx, bulkEditMessage(req.Msg.Fids, req.Msg.Operations), func() error {
		txns, err := h.hl.Transactions(ctx, journal.BuildFIDQuery(req.Msg.Fids))
		if err != nil {
			return fmt.Errorf("bulk-edit: lookup fids: %w", err)
		}
		byFID := make(map[string]hledger.Transaction, len(txns))
		countByFID := make(map[string]int, len(txns))
		for _, t := range txns {
			byFID[t.FID] = t
			countByFID[t.FID]++
		}

		items := make([]pendingEdit, 0, len(req.Msg.Fids))
		for _, fid := range req.Msg.Fids {
			t, ok := byFID[fid]
			if !ok {
				return fmt.Errorf("bulk-edit: no transaction found with fid %q: %w", fid, journal.ErrNotFound)
			}
			if countByFID[fid] > 1 {
				return fmt.Errorf("bulk-edit: fid %q matched %d transactions (corrupt journal — run audit)", fid, countByFID[fid])
			}
			src := journal.SourceLocation{File: t.SourcePos[0].File, Line: t.SourcePos[0].Line}
			input, err := journal.InputFromTransaction(t)
			if err != nil {
				return fmt.Errorf("bulk-edit: fid %q: %w", fid, err)
			}

			for _, op := range req.Msg.Operations {
				switch v := op.Operation.(type) {
				case *floatv1.BulkEditOperation_MarkReviewed:
					if v.MarkReviewed.Reviewed {
						input.Status = "Cleared"
					} else {
						input.Status = ""
					}
				case *floatv1.BulkEditOperation_AddTag:
					if input.Tags == nil {
						input.Tags = make(map[string]string)
					}
					input.Tags[v.AddTag.Key] = v.AddTag.Value
				case *floatv1.BulkEditOperation_RemoveTag:
					delete(input.Tags, v.RemoveTag.Key)
				case *floatv1.BulkEditOperation_SetPayee:
					note := t.Description // no "|": preserve full description as note
					if t.Note != nil {
						note = *t.Note
					}
					input.Description = v.SetPayee.Payee + " | " + note
				case *floatv1.BulkEditOperation_ClearPayee:
					// No "|" means no payee: leave the description untouched.
					if t.Note != nil {
						input.Description = *t.Note
					}
				case *floatv1.BulkEditOperation_UpdateUnknownAccount:
					newAcct := v.UpdateUnknownAccount.Account
					for i, p := range input.Postings {
						if strings.Contains(strings.ToLower(p.Account), "unknown") {
							input.Postings[i].Account = newAcct
						}
					}
					input.Status = "Cleared"
				}
			}

			items = append(items, pendingEdit{input: input, src: src, fid: fid})
		}

		inputs := make([]journal.TransactionInput, len(items))
		fids := make([]string, len(items))
		for i, it := range items {
			inputs[i] = it.input
			fids[i] = it.fid
		}
		texts, err := journal.BatchFormatViaHledger(ctx, h.hl, inputs, fids)
		if err != nil {
			return fmt.Errorf("bulk-edit: batch format: %w", err)
		}

		byFile := make(map[string][]journal.BatchReplacement)
		for i, it := range items {
			byFile[it.src.File] = append(byFile[it.src.File], journal.BatchReplacement{
				HeaderLine: it.src.Line,
				FID:        it.fid,
				NewText:    texts[i],
			})
		}
		for file, reps := range byFile {
			if err := journal.BatchReplaceTransactions(file, reps); err != nil {
				return fmt.Errorf("bulk-edit: batch replace in %s: %w", file, err)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "bulk edit transactions failed")
	}

	editedTxns, err := h.fetchByFIDs(ctx, req.Msg.Fids)
	if err != nil {
		return nil, rpcErr(ctx, err, "bulk edit: fetch after update failed")
	}
	editedByFID := make(map[string]*floatv1.Transaction, len(editedTxns))
	for _, t := range editedTxns {
		editedByFID[t.FID] = toProtoTransaction(t)
	}
	results := make([]*floatv1.Transaction, 0, len(req.Msg.Fids))
	for _, fid := range req.Msg.Fids {
		t, ok := editedByFID[fid]
		if !ok {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("transaction %s not found after bulk edit", fid))
		}
		results = append(results, t)
	}
	return connect.NewResponse(&floatv1.BulkEditTransactionsResponse{Transactions: results}), nil
}

func (h *Handler) BulkDeleteTransactions(
	ctx context.Context,
	req *connect.Request[floatv1.BulkDeleteTransactionsRequest],
	stream *connect.ServerStream[floatv1.BulkDeleteTransactionsResponse],
) error {
	if len(req.Msg.Fids) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("fids must not be empty"))
	}

	total := int32(len(req.Msg.Fids))
	var deleted int32

	err := h.lock.Do(ctx, fmt.Sprintf("delete %d transactions", len(req.Msg.Fids)), func() error {
		return journal.BatchDeleteTransactions(ctx, h.hl, req.Msg.Fids, func(d, _ int32) {
			deleted = d
			_ = stream.Send(&floatv1.BulkDeleteTransactionsResponse{
				Payload: &floatv1.BulkDeleteTransactionsResponse_Progress{
					Progress: &floatv1.BulkDeleteTransactionsProgress{
						Deleted: deleted,
						Total:   total,
					},
				},
			})
		})
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return connect.NewError(connect.CodeNotFound, err)
		}
		return rpcErr(ctx, err, "bulk delete transactions failed")
	}

	return stream.Send(&floatv1.BulkDeleteTransactionsResponse{
		Payload: &floatv1.BulkDeleteTransactionsResponse_Result{
			Result: &floatv1.BulkDeleteTransactionsResult{DeletedCount: deleted},
		},
	})
}

func bulkEditMessage(fids []string, ops []*floatv1.BulkEditOperation) string {
	if len(fids) == 1 && len(ops) == 1 {
		fid := fids[0]
		switch v := ops[0].Operation.(type) {
		case *floatv1.BulkEditOperation_MarkReviewed:
			if v.MarkReviewed.Reviewed {
				return "mark transaction " + fid + " as reviewed"
			}
			return "unmark transaction " + fid + " as reviewed"
		case *floatv1.BulkEditOperation_AddTag:
			return fmt.Sprintf("add tag %q to transaction %s", v.AddTag.Key, fid)
		case *floatv1.BulkEditOperation_RemoveTag:
			return fmt.Sprintf("remove tag %q from transaction %s", v.RemoveTag.Key, fid)
		case *floatv1.BulkEditOperation_SetPayee:
			return fmt.Sprintf("set payee on transaction %s", fid)
		case *floatv1.BulkEditOperation_ClearPayee:
			return "clear payee on transaction " + fid
		case *floatv1.BulkEditOperation_Delete:
			return "delete transaction " + fid
		case *floatv1.BulkEditOperation_UpdateUnknownAccount:
			return fmt.Sprintf("update unknown account to %q in transaction %s", v.UpdateUnknownAccount.Account, fid)
		}
	}
	if len(ops) == 1 {
		if _, ok := ops[0].Operation.(*floatv1.BulkEditOperation_Delete); ok {
			return fmt.Sprintf("delete %d transactions", len(fids))
		}
		if v, ok := ops[0].Operation.(*floatv1.BulkEditOperation_UpdateUnknownAccount); ok {
			return fmt.Sprintf("update unknown account to %q in %d transactions", v.UpdateUnknownAccount.Account, len(fids))
		}
	}
	return fmt.Sprintf("bulk edit %d transactions", len(fids))
}

func protoToJournalCost(c *floatv1.Cost) *journal.CostInput {
	if c == nil {
		return nil
	}
	return &journal.CostInput{
		Commodity: c.Commodity,
		Quantity:  c.Quantity,
		IsTotal:   c.IsTotal,
	}
}

func protoToJournalCostBasis(cb *floatv1.CostBasis) *journal.CostBasisInput {
	if cb == nil {
		return nil
	}
	out := &journal.CostBasisInput{
		Date:  cb.Date,
		Label: cb.Label,
	}
	if cb.Cost != nil {
		out.Commodity = cb.Cost.Commodity
		out.Quantity = cb.Cost.Quantity
	}
	return out
}

// protoToJournalAssertion converts a proto BalanceAssertion to the journal
// input form. The gRPC API only exposes the simple = form, so Inclusive
// and Total are always false here.
func protoToJournalAssertion(ba *floatv1.BalanceAssertion) *journal.BalanceAssertionInput {
	if ba == nil || ba.Amount == nil {
		return nil
	}
	return &journal.BalanceAssertionInput{
		Commodity: ba.Amount.Commodity,
		Quantity:  ba.Amount.Quantity,
	}
}
