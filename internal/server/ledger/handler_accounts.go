package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/config"
	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/journal"
	"github.com/brendanv/float/internal/rules"
	"github.com/brendanv/float/internal/slogctx"
	"github.com/brendanv/float/internal/templates"
	"github.com/brendanv/float/internal/txlock"
)

func (h *Handler) ListAccounts(ctx context.Context, req *connect.Request[floatv1.ListAccountsRequest]) (*connect.Response[floatv1.ListAccountsResponse], error) {
	nodes, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger accounts failed")
	}
	closed, err := cachedClosedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger closed accounts failed")
	}
	accounts := make([]*floatv1.Account, len(nodes))
	for i, n := range nodes {
		accounts[i] = toProtoAccount(n)
		accounts[i].Closed = closed[n.FullName]
	}
	return connect.NewResponse(&floatv1.ListAccountsResponse{Accounts: accounts}), nil
}

// GetBalanceAssertionStatus reports, for every asset and liability account that
// has postings, the date of its most recent balance assertion, the number of
// transactions since that assertion, and its most recent transaction (so the UI
// can edit it to add a fresh assertion). It does a single transactions pass and
// groups in Go rather than running hledger once per account. Accounts are sorted
// by the transaction count since the last assertion so the busiest drift risks
// surface first.
func (h *Handler) GetBalanceAssertionStatus(ctx context.Context, req *connect.Request[floatv1.GetBalanceAssertionStatusRequest]) (*connect.Response[floatv1.GetBalanceAssertionStatusResponse], error) {
	nodes, err := cachedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger accounts failed")
	}
	closed, err := cachedClosedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger closed accounts failed")
	}
	// Cash (C) is an hledger subtype of Asset, so treat it as an asset too.
	// Closed accounts are skipped: there is nothing left to reconcile.
	accountType := make(map[string]hledger.AccountType, len(nodes))
	for _, n := range nodes {
		if closed[n.FullName] {
			continue
		}
		switch n.Type {
		case hledger.AccountTypeAsset, hledger.AccountTypeCash:
			accountType[n.FullName] = hledger.AccountTypeAsset
		case hledger.AccountTypeLiability:
			accountType[n.FullName] = hledger.AccountTypeLiability
		}
	}

	txns, err := cachedTransactions(ctx, h.cache, h.hl, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger transactions failed")
	}

	balReport, err := cachedBalances(ctx, h.cache, h.hl, hledger.BalanceOpts{}, nil)
	if err != nil {
		return nil, rpcErr(ctx, err, "hledger balances failed")
	}
	balByAccount := make(map[string][]hledger.Amount, len(balReport.Rows))
	for _, row := range balReport.Rows {
		balByAccount[row.FullName] = row.Amounts
	}

	type agg struct {
		lastTxn               *hledger.Transaction
		lastAssertion         string // "YYYY-MM-DD"; "" if never asserted
		transactionCount      int32
		lastAssertionTxnCount int32
	}
	aggs := make(map[string]*agg)
	for i := range txns {
		t := &txns[i]
		seen := make(map[string]bool)
		for _, p := range t.Postings {
			if _, ok := accountType[p.Account]; !ok {
				continue
			}
			a := aggs[p.Account]
			if a == nil {
				a = &agg{}
				aggs[p.Account] = a
			}
			// Transactions arrive in ascending date/file order; track the most
			// recent transaction touching each account (compare dates so we are
			// resilient to unsorted input).
			if !seen[p.Account] {
				a.transactionCount++
				if a.lastTxn == nil || t.Date >= a.lastTxn.Date {
					a.lastTxn = t
				}
				seen[p.Account] = true
			}
			// Any assertion form (=, =*, ==, ==*) counts as "asserted".
			if p.BalanceAssertion != nil && t.Date >= a.lastAssertion {
				a.lastAssertion = t.Date
				a.lastAssertionTxnCount = a.transactionCount
			}
		}
	}

	statuses := make([]*floatv1.AccountAssertionStatus, 0, len(aggs))
	for account, a := range aggs {
		if a.lastTxn == nil {
			continue
		}
		transactionsSinceLastAssertion := a.transactionCount
		if a.lastAssertion != "" {
			transactionsSinceLastAssertion -= a.lastAssertionTxnCount
		}
		s := &floatv1.AccountAssertionStatus{
			Account:                        account,
			Type:                           string(accountType[account]),
			Balance:                        aggregateAmountsByCommodity(balByAccount[account]),
			LastTransaction:                toProtoTransaction(*a.lastTxn),
			TransactionsSinceLastAssertion: transactionsSinceLastAssertion,
		}
		if a.lastAssertion != "" {
			s.LastAssertionDate = &a.lastAssertion
		}
		statuses = append(statuses, s)
	}

	sort.Slice(statuses, func(i, j int) bool {
		ti := statuses[i].GetTransactionsSinceLastAssertion()
		tj := statuses[j].GetTransactionsSinceLastAssertion()
		if ti != tj {
			return ti > tj
		}
		return statuses[i].Account < statuses[j].Account
	})

	return connect.NewResponse(&floatv1.GetBalanceAssertionStatusResponse{Accounts: statuses}), nil
}

func (h *Handler) ListAccountDeclarations(ctx context.Context, _ *connect.Request[floatv1.ListAccountDeclarationsRequest]) (*connect.Response[floatv1.ListAccountDeclarationsResponse], error) {
	decls, err := journal.ListAccountDeclarations(h.dataDir)
	if err != nil {
		return nil, rpcErr(ctx, err, "list account declarations failed")
	}
	unused, err := cachedUnusedAccounts(ctx, h.cache, h.hl)
	if err != nil {
		return nil, rpcErr(ctx, err, "unused accounts failed")
	}
	unusedSet := make(map[string]bool, len(unused))
	for _, name := range unused {
		unusedSet[name] = true
	}
	out := make([]*floatv1.AccountDeclaration, len(decls))
	for i, d := range decls {
		pd := toProtoAccountDeclaration(d)
		pd.HasPostings = !unusedSet[d.Name]
		out[i] = pd
	}
	return connect.NewResponse(&floatv1.ListAccountDeclarationsResponse{Declarations: out}), nil
}

func (h *Handler) DeclareAccount(ctx context.Context, req *connect.Request[floatv1.DeclareAccountRequest]) (*connect.Response[floatv1.DeclareAccountResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("declare account: %s", name), func() error {
		return journal.AppendAccountDeclaration(h.dataDir, name)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "declare account failed", "name", name)
	}
	return connect.NewResponse(&floatv1.DeclareAccountResponse{
		Declaration: toProtoAccountDeclaration(journal.AccountDeclaration{Name: name}),
	}), nil
}

func (h *Handler) DeleteAccountDeclaration(ctx context.Context, req *connect.Request[floatv1.DeleteAccountDeclarationRequest]) (*connect.Response[floatv1.DeleteAccountDeclarationResponse], error) {
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	err := h.lock.Do(ctx, fmt.Sprintf("delete account declaration %s", req.Msg.Name), func() error {
		return journal.DeleteAccountDeclaration(h.dataDir, req.Msg.Name)
	})
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, rpcErr(ctx, err, "delete account declaration failed", "name", req.Msg.Name)
	}
	return connect.NewResponse(&floatv1.DeleteAccountDeclarationResponse{}), nil
}

func (h *Handler) SetAccountClosed(ctx context.Context, req *connect.Request[floatv1.SetAccountClosedRequest]) (*connect.Response[floatv1.SetAccountClosedResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	verb := "reopen"
	if req.Msg.Closed {
		verb = "close"
	}
	err := h.lock.Do(ctx, fmt.Sprintf("%s account: %s", verb, name), func() error {
		return journal.SetAccountClosed(h.dataDir, name, req.Msg.Closed)
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "set account closed failed", "name", name, "closed", req.Msg.Closed)
	}
	return connect.NewResponse(&floatv1.SetAccountClosedResponse{}), nil
}

func (h *Handler) RenameAccount(ctx context.Context, req *connect.Request[floatv1.RenameAccountRequest]) (*connect.Response[floatv1.RenameAccountResponse], error) {
	oldName := strings.TrimSpace(req.Msg.OldName)
	newName := strings.TrimSpace(req.Msg.NewName)
	if oldName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("old_name is required"))
	}
	if newName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("new_name is required"))
	}
	if oldName == newName {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("old_name and new_name must differ"))
	}

	// Besides the journal, float's own config references accounts: rule
	// targets/filters (rules.json), template postings (templates.json), and
	// Stripe linked accounts / portfolio exclusions (config.toml). Leaving those
	// stale would keep creating postings to the old account, so they are
	// renamed in the same locked write and reverted with it on failure.
	extraPaths := []string{rules.FilePath(h.dataDir), templates.FilePath(h.dataDir)}
	if h.configPath != "" {
		extraPaths = append(extraPaths, h.configPath)
	}

	var (
		renamed int
		baseCfg *config.Config // in-memory config the rename was applied to
		newCfg  *config.Config // renamed config saved to disk; nil if unchanged
	)
	msg := fmt.Sprintf("rename account: %s → %s", oldName, newName)
	err := h.lock.DoWith(ctx, msg, extraPaths, func() error {
		// Rename in accounts.journal if the declaration exists there.
		if declErr := journal.RenameAccountDeclaration(h.dataDir, oldName, newName); declErr != nil {
			if !errors.Is(declErr, journal.ErrNotFound) {
				return declErr
			}
		}
		// Rename in all transaction (YYYY/MM) journal files.
		n, err := journal.RenameAccountInJournalFiles(h.dataDir, oldName, newName)
		if err != nil {
			return err
		}
		renamed = n

		if err := renameAccountInRules(h.dataDir, oldName, newName); err != nil {
			return err
		}
		if err := renameAccountInTemplates(h.dataDir, oldName, newName); err != nil {
			return err
		}
		baseCfg = h.loadCfg()
		if baseCfg != nil && h.configPath != "" {
			if c, changed := renameAccountInConfig(baseCfg, oldName, newName); changed {
				if err := config.Save(h.configPath, c); err != nil {
					return err
				}
				newCfg = c
			}
		}
		return nil
	})
	if err != nil {
		return nil, rpcErr(ctx, err, "rename account failed", "old", oldName, "new", newName)
	}

	// Publish the renamed config only now: DoWith runs `hledger check` after fn
	// and reverts config.toml on failure, so storing it inside fn could leave
	// memory out of sync with disk.
	if newCfg != nil && !h.cfg.CompareAndSwap(baseCfg, newCfg) {
		// Another config writer locked and published between our unlock and the
		// swap. It started from the pre-rename config and saved that to disk, so
		// re-apply the rename on top of its result.
		h.reapplyConfigRename(ctx, msg, oldName, newName)
	}
	return connect.NewResponse(&floatv1.RenameAccountResponse{PostingsRenamed: int32(renamed)}), nil
}

// reapplyConfigRename re-applies an account rename to the current config under
// the lock. Only config.toml is touched, so DoConfig is sufficient. Failures
// are logged rather than returned: the journal rename already committed.
func (h *Handler) reapplyConfigRename(ctx context.Context, msg, oldName, newName string) {
	err := h.lock.DoConfig(ctx, msg+" (config)", []string{h.configPath}, func() error {
		c, changed := renameAccountInConfig(h.loadCfg(), oldName, newName)
		if !changed {
			return txlock.ErrNoChanges
		}
		if err := config.Save(h.configPath, c); err != nil {
			return err
		}
		h.cfg.Store(c)
		return nil
	})
	if err != nil {
		slogctx.FromContext(ctx).ErrorContext(ctx, "rename account: re-applying config rename failed", "old", oldName, "new", newName, "error", err)
	}
}

// renameAccountInRules renames rule target accounts and source-account filters
// that are oldName or one of its sub-accounts. Does nothing (and does not
// create rules.json) when no rule references the account. Must run inside the
// txlock with rules.FilePath declared.
func renameAccountInRules(dataDir, oldName, newName string) error {
	list, err := rules.Load(dataDir)
	if err != nil {
		return err
	}
	changed := false
	for i := range list {
		if a, ok := journal.RenameAccountName(list[i].Account, oldName, newName); ok {
			list[i].Account = a
			changed = true
		}
		if a, ok := journal.RenameAccountName(list[i].MatchAccount, oldName, newName); ok {
			list[i].MatchAccount = a
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return rules.Save(dataDir, list)
}

// renameAccountInTemplates renames template posting accounts that are oldName
// or one of its sub-accounts. Does nothing (and does not create
// templates.json) when no template references the account. Must run inside
// the txlock with templates.FilePath declared.
func renameAccountInTemplates(dataDir, oldName, newName string) error {
	list, err := templates.Load(dataDir)
	if err != nil {
		return err
	}
	changed := false
	for i := range list {
		for j := range list[i].Postings {
			if a, ok := journal.RenameAccountName(list[i].Postings[j].Account, oldName, newName); ok {
				list[i].Postings[j].Account = a
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	return templates.Save(dataDir, list)
}

// renameAccountInConfig returns a copy of cfg with Stripe linked accounts'
// HledgerAccount, portfolio excluded account prefixes and the transfers
// clearing account renamed where they are oldName or one of its sub-accounts. cfg itself is never modified (it is
// shared, copy-on-write state). Returns (cfg, false) when nothing matches.
func renameAccountInConfig(cfg *config.Config, oldName, newName string) (*config.Config, bool) {
	if cfg == nil {
		return nil, false
	}
	changed := false

	linked := slices.Clone(cfg.Stripe.LinkedAccounts)
	for i := range linked {
		if a, ok := journal.RenameAccountName(linked[i].HledgerAccount, oldName, newName); ok {
			linked[i].HledgerAccount = a
			changed = true
		}
	}

	excluded := slices.Clone(cfg.Portfolio.ExcludedAccountPrefixes)
	for i := range excluded {
		if a, ok := journal.RenameAccountName(excluded[i], oldName, newName); ok {
			excluded[i] = a
			changed = true
		}
	}

	// Rename the effective clearing account, so renaming the default
	// equity:transfers while config leaves it unset keeps the check on it.
	transfersAccount := cfg.Transfers.Account
	if a, ok := journal.RenameAccountName(transfersAccountOf(cfg), oldName, newName); ok {
		transfersAccount = a
		changed = true
	}

	if !changed {
		return cfg, false
	}
	out := *cfg
	out.Stripe.LinkedAccounts = linked
	out.Portfolio.ExcludedAccountPrefixes = excluded
	out.Transfers.Account = transfersAccount
	return &out, true
}
