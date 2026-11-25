package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
)

// ---- Rules handlers ----

func (h *Handler) ListRules(ctx context.Context, _ *connect.Request[floatv1.ListRulesRequest]) (*connect.Response[floatv1.ListRulesResponse], error) {
	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "list rules failed")
	}
	out := make([]*floatv1.TransactionRule, len(rulesList))
	for i, r := range rulesList {
		out[i] = toProtoRule(r)
	}
	return connect.NewResponse(&floatv1.ListRulesResponse{Rules: out}), nil
}

func (h *Handler) AddRule(ctx context.Context, req *connect.Request[floatv1.AddRuleRequest]) (*connect.Response[floatv1.AddRuleResponse], error) {
	if len(req.Msg.Rules) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("at least one rule is required"))
	}
	patterns := make([]string, len(req.Msg.Rules))
	for i, r := range req.Msg.Rules {
		if r.Pattern == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pattern is required"))
		}
		if _, err := rules.CompilePattern(r.Pattern); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pattern %q: %w", r.Pattern, err))
		}
		patterns[i] = r.Pattern
	}

	var newRules []rules.Rule
	err := h.lock.DoConfig(ctx, addRuleMessage(patterns), []string{rules.FilePath(h.dataDir)}, func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		newRules = make([]rules.Rule, len(req.Msg.Rules))
		for i, r := range req.Msg.Rules {
			newRules[i] = rules.Rule{
				ID:           journal.MintFID(),
				Pattern:      r.Pattern,
				Payee:        r.Payee,
				Account:      r.Account,
				Tags:         r.Tags,
				Priority:     int(r.Priority),
				AutoReviewed: r.AutoReviewed,
				MatchAccount: r.MatchAccount,
			}
		}
		rulesList = append(rulesList, newRules...)
		return rules.Save(h.dataDir, rulesList)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "add rule failed")
	}
	out := make([]*floatv1.TransactionRule, len(newRules))
	for i, r := range newRules {
		out[i] = toProtoRule(r)
	}
	return connect.NewResponse(&floatv1.AddRuleResponse{Rules: out}), nil
}

const maxSnapshotDescriptionLen = 180

func addRuleMessage(patterns []string) string {
	prefix := fmt.Sprintf("add %d rule(s)", len(patterns))
	if len(patterns) == 0 {
		return prefix
	}
	full := prefix + ": " + strings.Join(patterns, ", ")
	if len(full) <= maxSnapshotDescriptionLen {
		return full
	}

	base := prefix + ": "
	remaining := maxSnapshotDescriptionLen - len(base)
	if remaining <= 0 {
		return prefix
	}

	var b strings.Builder
	used := 0
	appended := 0
	for i, pattern := range patterns {
		part := pattern
		if appended > 0 {
			part = ", " + pattern
		}
		if used+len(part) > remaining {
			left := len(patterns) - i
			if left > 0 {
				suffix := fmt.Sprintf(" ... +%d more", left)
				for appended > 0 && used+len(suffix) > remaining {
					text := b.String()
					last := strings.LastIndex(text, ", ")
					if last < 0 {
						b.Reset()
						used = 0
						break
					}
					b.Reset()
					b.WriteString(text[:last])
					used = len(text[:last])
					appended--
				}
				if used+len(suffix) <= remaining {
					b.WriteString(suffix)
				}
			}
			break
		}
		b.WriteString(part)
		used += len(part)
		appended++
	}
	if b.Len() == 0 {
		return prefix
	}
	return base + b.String()
}

func (h *Handler) UpdateRule(ctx context.Context, req *connect.Request[floatv1.UpdateRuleRequest]) (*connect.Response[floatv1.UpdateRuleResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if req.Msg.Pattern == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("pattern is required"))
	}
	if _, err := rules.CompilePattern(req.Msg.Pattern); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pattern %q: %w", req.Msg.Pattern, err))
	}

	var updated rules.Rule
	err := h.lock.DoConfig(ctx, fmt.Sprintf("update rule %s", req.Msg.Id), []string{rules.FilePath(h.dataDir)}, func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		found := false
		for i, r := range rulesList {
			if r.ID == req.Msg.Id {
				rulesList[i] = rules.Rule{
					ID:           req.Msg.Id,
					Pattern:      req.Msg.Pattern,
					Payee:        req.Msg.Payee,
					Account:      req.Msg.Account,
					Tags:         req.Msg.Tags,
					Priority:     int(req.Msg.Priority),
					AutoReviewed: req.Msg.AutoReviewed,
					MatchAccount: req.Msg.MatchAccount,
				}
				updated = rulesList[i]
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("rule %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return rules.Save(h.dataDir, rulesList)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "update rule failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(&floatv1.UpdateRuleResponse{Rule: toProtoRule(updated)}), nil
}

func (h *Handler) DeleteRule(ctx context.Context, req *connect.Request[floatv1.DeleteRuleRequest]) (*connect.Response[floatv1.DeleteRuleResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	err := h.lock.DoConfig(ctx, fmt.Sprintf("delete rule %s", req.Msg.Id), []string{rules.FilePath(h.dataDir)}, func() error {
		rulesList, loadErr := rules.Load(h.dataDir)
		if loadErr != nil {
			return loadErr
		}
		filtered := rulesList[:0]
		found := false
		for _, r := range rulesList {
			if r.ID == req.Msg.Id {
				found = true
				continue
			}
			filtered = append(filtered, r)
		}
		if !found {
			return fmt.Errorf("rule %q: %w", req.Msg.Id, journal.ErrNotFound)
		}
		return rules.Save(h.dataDir, filtered)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete rule failed", "id", req.Msg.Id)
	}
	return connect.NewResponse(&floatv1.DeleteRuleResponse{}), nil
}

func (h *Handler) PreviewApplyRules(ctx context.Context, req *connect.Request[floatv1.PreviewApplyRulesRequest]) (*connect.Response[floatv1.PreviewApplyRulesResponse], error) {

	rulesList, err := rules.Load(h.dataDir)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Filter to requested rules if specified.
	if len(req.Msg.RuleIds) > 0 {
		rulesList = filterRules(rulesList, req.Msg.RuleIds)
	}

	txns, err := cachedTransactions(ctx, h.cache, h.hl, req.Msg.Query)
	if err != nil {
		return nil, rpcErr(ctx, err, "fetch transactions failed")
	}

	matches := rules.Preview(rulesList, txns)
	previews := make([]*floatv1.RuleApplicationPreview, len(matches))
	for i, m := range matches {
		p := &floatv1.RuleApplicationPreview{
			Fid:           m.Transaction.FID,
			Description:   m.Transaction.Description,
			MatchedRuleId: m.Rule.ID,
			AddTags:       m.Changes.AddTags,
		}
		// Current category account.
		if idx := rules.CategoryPostingIndex(m.Transaction); idx >= 0 {
			p.CurrentAccount = m.Transaction.Postings[idx].Account
		}
		if m.Transaction.Payee != nil {
			p.CurrentPayee = *m.Transaction.Payee
		}
		if m.Changes.NewAccount != nil {
			p.NewAccount = *m.Changes.NewAccount
		}
		if m.Changes.NewPayee != nil {
			p.NewPayee = *m.Changes.NewPayee
		}
		if m.Changes.MarkReviewed != nil && *m.Changes.MarkReviewed {
			p.WillMarkReviewed = true
		}
		previews[i] = p
	}
	return connect.NewResponse(&floatv1.PreviewApplyRulesResponse{Previews: previews}), nil
}

func (h *Handler) ApplyRules(ctx context.Context, req *connect.Request[floatv1.ApplyRulesRequest], stream *connect.ServerStream[floatv1.ApplyRulesResponse]) error {
	if h.beforeApplyRulesLock != nil {
		h.beforeApplyRulesLock()
	}

	// Load rules, fetch transactions, and compute matches inside the lock:
	// ApplyBatch rewrites each matched transaction from these parsed values at
	// their source line, so any write landing between a pre-lock fetch and
	// lock acquisition would be silently overwritten with stale content.
	// h.hl.Transactions bypasses the cache, which must not serve in-lock reads.
	var total, applied int32
	err := h.lock.Do(ctx, "apply rules", func() error {
		rulesList, err := rules.Load(h.dataDir)
		if err != nil {
			return fmt.Errorf("load rules: %w", err)
		}
		if len(req.Msg.RuleIds) > 0 {
			rulesList = filterRules(rulesList, req.Msg.RuleIds)
		}

		txns, err := h.hl.Transactions(ctx, req.Msg.Query...)
		if err != nil {
			return fmt.Errorf("fetch transactions: %w", err)
		}

		matches := rules.Preview(rulesList, txns)

		// Filter to requested FIDs if specified.
		if len(req.Msg.Fids) > 0 {
			fidSet := make(map[string]bool, len(req.Msg.Fids))
			for _, fid := range req.Msg.Fids {
				fidSet[fid] = true
			}
			filtered := matches[:0]
			for _, m := range matches {
				if fidSet[m.Transaction.FID] {
					filtered = append(filtered, m)
				}
			}
			matches = filtered
		}

		total = int32(len(matches))
		return rules.ApplyBatch(ctx, h.hl, h.dataDir, matches, func(a, _ int32) {
			applied = a
			_ = stream.Send(&floatv1.ApplyRulesResponse{
				Payload: &floatv1.ApplyRulesResponse_Progress{
					Progress: &floatv1.ApplyRulesProgress{
						Applied: applied,
						Total:   total,
					},
				},
			})
		})
	})
	if err != nil {
		return rpcErr(ctx, err, "apply rules failed")
	}

	return stream.Send(&floatv1.ApplyRulesResponse{
		Payload: &floatv1.ApplyRulesResponse_Result{
			Result: &floatv1.ApplyRulesResult{AppliedCount: applied},
		},
	})
}

// filterRules returns only rules whose IDs are in the given set.
func filterRules(rulesList []rules.Rule, ids []string) []rules.Rule {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	filtered := rulesList[:0]
	for _, r := range rulesList {
		if idSet[r.ID] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func toProtoRule(r rules.Rule) *floatv1.TransactionRule {
	return &floatv1.TransactionRule{
		Id:           r.ID,
		Pattern:      r.Pattern,
		Payee:        r.Payee,
		Account:      r.Account,
		Tags:         r.Tags,
		Priority:     int32(r.Priority),
		AutoReviewed: r.AutoReviewed,
		MatchAccount: r.MatchAccount,
	}
}
