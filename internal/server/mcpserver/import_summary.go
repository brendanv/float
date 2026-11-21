package mcpserver

import (
	"context"
	"net/http"
	"slices"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
)

type importSummary struct {
	DryRun                bool         `json:"dry_run,omitempty"`
	ImportBatchID         string       `json:"import_batch_id,omitempty"`
	Count                 int          `json:"count"`
	ImportedCount         int32        `json:"imported_count,omitempty"`
	SkippedDuplicateCount int32        `json:"skipped_duplicate_count,omitempty"`
	Duplicates            []int        `json:"duplicates,omitempty"`
	FIDs                  []string     `json:"fids,omitempty"`
	RuleMatches           []ruleEffect `json:"rule_matches,omitempty"`
}

type ruleEffect struct {
	Index    int               `json:"index"`
	RuleID   string            `json:"rule_id"`
	Payee    string            `json:"payee,omitempty"`
	Accounts []string          `json:"accounts,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
	Status   string            `json:"status,omitempty"`
}

func importStructured(c floatv1connect.LedgerServiceClient) rpc {
	return rpc{
		input: (&floatv1.ImportStructuredTransactionsRequest{}).ProtoReflect().Descriptor(),
		call: func(ctx context.Context, header http.Header, args []byte) (any, error) {
			req, err := decodeRequest[floatv1.ImportStructuredTransactionsRequest, *floatv1.ImportStructuredTransactionsRequest](header, args)
			if err != nil {
				return nil, err
			}
			resp, err := c.ImportStructuredTransactions(ctx, req)
			if err != nil {
				return nil, err
			}
			return summarizeImport(req.Msg, resp.Msg), nil
		},
	}
}

func summarizeImport(req *floatv1.ImportStructuredTransactionsRequest, resp *floatv1.ImportStructuredTransactionsResponse) importSummary {
	s := importSummary{
		DryRun:                req.DryRun,
		ImportBatchID:         resp.ImportBatchId,
		Count:                 len(resp.Candidates),
		ImportedCount:         resp.ImportedCount,
		SkippedDuplicateCount: resp.SkippedDuplicateCount,
	}
	var written []int
	for i, cand := range resp.Candidates {
		if cand.IsDuplicate {
			s.Duplicates = append(s.Duplicates, i)
		}
		if !req.DryRun && (!cand.IsDuplicate || req.IncludeDuplicates) {
			written = append(written, i)
		}
		if cand.MatchedRuleId != "" && i < len(req.Transactions) {
			s.RuleMatches = append(s.RuleMatches, ruleEffectOf(i, cand, req.Transactions[i]))
		}
	}
	if len(written) > 0 && len(written) == len(resp.Transactions) {
		s.FIDs = make([]string, len(resp.Candidates))
		for j, i := range written {
			s.FIDs[i] = resp.Transactions[j].Fid
		}
	}
	return s
}

func ruleEffectOf(i int, cand *floatv1.ImportCandidate, in *floatv1.StructuredTransaction) ruleEffect {
	e := ruleEffect{Index: i, RuleID: cand.MatchedRuleId}
	t := cand.Transaction
	if t == nil {
		return e
	}
	if p := t.GetPayee(); p != "" && p != in.Payee {
		e.Payee = p
	}
	inAccounts := make([]string, len(in.Postings))
	for j, p := range in.Postings {
		inAccounts[j] = p.Account
	}
	for _, p := range t.Postings {
		if !slices.Contains(inAccounts, p.Account) && !slices.Contains(e.Accounts, p.Account) {
			e.Accounts = append(e.Accounts, p.Account)
		}
	}
	for k, v := range t.Tags {
		if _, ok := in.Tags[k]; !ok {
			if e.Tags == nil {
				e.Tags = map[string]string{}
			}
			e.Tags[k] = v
		}
	}
	if t.Status != in.Status {
		e.Status = t.Status
	}
	return e
}
