package ledger

import (
	"fmt"
	"strings"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
)

func toProtoTransaction(t hledger.Transaction) *floatv1.Transaction {
	postings := make([]*floatv1.Posting, len(t.Postings))
	for i, p := range t.Postings {
		postings[i] = toProtoPosting(p)
	}
	// Normalize hledger's "Unmarked" to "" for consistency with the proto contract.
	status := t.Status
	if status == "Unmarked" {
		status = ""
	}
	tags := make(map[string]string, len(t.Tags))
	for _, kv := range t.Tags {
		if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
			tags[kv[0]] = kv[1]
		}
	}
	var importBatchID *string
	if v, ok := t.FloatMeta["float-import"]; ok {
		importBatchID = &v
	}
	var stripeTxnID *string
	if v, ok := t.FloatMeta["float-stripe-txn"]; ok {
		stripeTxnID = &v
	}
	return &floatv1.Transaction{
		Fid:                 t.FID,
		Date:                t.Date,
		Description:         t.Description,
		Comment:             t.Comment,
		Postings:            postings,
		Status:              status,
		Tags:                tags,
		Payee:               t.Payee,
		Note:                t.Note,
		ImportBatchId:       importBatchID,
		StripeTransactionId: stripeTxnID,
	}
}

func toProtoPosting(p hledger.Posting) *floatv1.Posting {
	amounts := make([]*floatv1.Amount, len(p.Amounts))
	for i, a := range p.Amounts {
		amounts[i] = toProtoAmount(a)
	}
	return &floatv1.Posting{
		Account:          p.Account,
		Amounts:          amounts,
		Comment:          p.Comment,
		BalanceAssertion: toProtoBalanceAssertion(p.BalanceAssertion),
		LotRole:          p.LotRole(),
		Generated:        p.Generated(),
	}
}

func toProtoLot(r hledger.LotRow) *floatv1.Lot {
	out := &floatv1.Lot{
		Account:   r.Account,
		LotName:   r.LotName,
		Date:      r.Date,
		Label:     r.Label,
		Units:     toProtoAmount(r.Units),
		TotalCost: toProtoAmounts(r.TotalCost),
	}
	if r.UnitCost != nil {
		out.UnitCost = toProtoAmount(*r.UnitCost)
	}
	return out
}

func toProtoCostBasis(cb *hledger.CostBasis) *floatv1.CostBasis {
	if cb == nil {
		return nil
	}
	out := &floatv1.CostBasis{}
	if cb.Date != nil {
		out.Date = *cb.Date
	}
	if cb.Label != nil {
		out.Label = *cb.Label
	}
	if cb.Cost != nil {
		out.Cost = toProtoAmount(*cb.Cost)
	}
	return out
}

// toProtoBalanceAssertion exposes only the simple = form via gRPC.
// Non-`=` variants (=*, ==, ==*) are preserved on disk by the journal
// package but hidden from API responses.
func toProtoBalanceAssertion(ba *hledger.BalanceAssertion) *floatv1.BalanceAssertion {
	if ba == nil || ba.Inclusive || ba.Total {
		return nil
	}
	return &floatv1.BalanceAssertion{
		Amount: toProtoAmount(ba.Amount),
	}
}

func toProtoAmount(a hledger.Amount) *floatv1.Amount {
	out := &floatv1.Amount{
		Commodity: a.Commodity,
		Quantity:  a.Quantity.Format(),
	}
	if cost, err := a.ParseCost(); err == nil && cost != nil {
		out.Cost = &floatv1.Cost{
			Commodity: cost.Contents.Commodity,
			Quantity:  cost.Contents.Quantity.Format(),
			IsTotal:   cost.Tag == "TotalCost",
		}
	}
	out.CostBasis = toProtoCostBasis(a.CostBasis)
	return out
}

func toProtoAccountRegisterRow(r hledger.AregisterRow) *floatv1.AccountRegisterRow {
	change := make([]*floatv1.Amount, len(r.Change))
	for i, a := range r.Change {
		change[i] = toProtoAmount(a)
	}
	balance := make([]*floatv1.Amount, len(r.Balance))
	for i, a := range r.Balance {
		balance[i] = toProtoAmount(a)
	}
	// Normalize "Unmarked" to "" for proto contract, matching toProtoTransaction.
	status := r.Transaction.Status
	if status == "Unmarked" {
		status = ""
	}
	tags := make(map[string]string, len(r.Transaction.Tags))
	for _, kv := range r.Transaction.Tags {
		if !strings.HasPrefix(kv[0], hledger.HiddenMetaPrefix) {
			tags[kv[0]] = kv[1]
		}
	}
	var stripeTxnID *string
	if v, ok := r.Transaction.FloatMeta["float-stripe-txn"]; ok {
		stripeTxnID = &v
	}
	row := &floatv1.AccountRegisterRow{
		Fid:                 r.Transaction.FID,
		Date:                r.Transaction.Date,
		Description:         r.Transaction.Description,
		Status:              status,
		OtherAccounts:       append([]string(nil), r.OtherAccounts...),
		Change:              change,
		RunningTotal:        balance,
		Tags:                tags,
		StripeTransactionId: stripeTxnID,
	}
	row.Payee = r.Transaction.Payee
	row.Note = r.Transaction.Note
	return row
}

func toProtoBalanceRow(r hledger.BalanceRow) *floatv1.BalanceRow {
	amounts := make([]*floatv1.Amount, len(r.Amounts))
	for i, a := range r.Amounts {
		amounts[i] = toProtoAmount(a)
	}
	return &floatv1.BalanceRow{
		DisplayName: r.DisplayName,
		FullName:    r.FullName,
		Indent:      int32(r.Indent),
		Amounts:     amounts,
	}
}

func toProtoAccount(n *hledger.AccountNode) *floatv1.Account {
	return &floatv1.Account{
		Name:     n.Name,
		FullName: n.FullName,
		Type:     string(n.Type),
	}
}

func toProtoAmounts(amounts []hledger.Amount) []*floatv1.Amount {
	result := make([]*floatv1.Amount, len(amounts))
	for i, a := range amounts {
		result[i] = toProtoAmount(a)
	}
	return result
}

// aggregateAmountsByCommodity sums amounts that share the same commodity.
// Zero-balance commodities are omitted from the result.
func aggregateAmountsByCommodity(amounts []hledger.Amount) []*floatv1.Amount {
	totals := make(map[string]hledger.AmountQuantity)
	var order []string
	for _, a := range amounts {
		if _, seen := totals[a.Commodity]; !seen {
			order = append(order, a.Commodity)
		}
		totals[a.Commodity] = totals[a.Commodity].Add(a.Quantity)
	}
	var result []*floatv1.Amount
	for _, commodity := range order {
		eq := totals[commodity]
		if eq.DecimalMantissa == 0 {
			continue
		}
		result = append(result, &floatv1.Amount{
			Commodity: commodity,
			Quantity:  fmt.Sprintf("%g", eq.FloatingPoint),
		})
	}
	return result
}

func toProtoAccountDeclaration(d journal.AccountDeclaration) *floatv1.AccountDeclaration {
	return &floatv1.AccountDeclaration{
		Name:   d.Name,
		Closed: d.Closed,
	}
}

func toProtoPriceDirective(p journal.Price) *floatv1.PriceDirective {
	return &floatv1.PriceDirective{
		Pid:       p.PID,
		Date:      p.Date,
		Commodity: p.Commodity,
		Price: &floatv1.Amount{
			Commodity: p.Currency,
			Quantity:  p.Quantity,
		},
	}
}
