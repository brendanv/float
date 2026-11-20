package mcpserver

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// defaultPageSize is the limit list tools apply when the caller leaves it at
// 0, which the RPCs treat as "no limit". An unbounded transaction list can
// run to megabytes, far past what an agent can read.
const defaultPageSize = 100

// effect classifies what a tool does, for MCP tool annotations.
type effect int

const (
	readOnly effect = iota
	// additive writes add or update data without discarding any.
	additive
	// destructive writes may delete or overwrite existing data.
	destructive
)

// tool is one curated LedgerService RPC exposed over MCP.
type tool struct {
	name        string
	title       string
	description string
	effect      effect
	// idempotent: repeating the call with the same arguments has no further
	// effect.
	idempotent bool
	rpc        rpc
}

// rpc decodes tool arguments into a request message, calls the RPC with the
// given request headers and returns the response message.
type rpc struct {
	input protoreflect.MessageDescriptor
	call  func(ctx context.Context, header http.Header, args []byte) (proto.Message, error)
}

type protoPtr[T any] interface {
	*T
	proto.Message
}

// unary adapts a generated unary client method. prep, if set, fills request
// defaults after decoding.
func unary[Req, Res any, PReq protoPtr[Req], PRes protoPtr[Res]](
	fn func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error),
	prep func(PReq),
) rpc {
	return rpc{
		input: PReq(new(Req)).ProtoReflect().Descriptor(),
		call: func(ctx context.Context, header http.Header, args []byte) (proto.Message, error) {
			msg := PReq(new(Req))
			if err := decodeArgs(args, msg); err != nil {
				return nil, err
			}
			if prep != nil {
				prep(msg)
			}
			req := connect.NewRequest((*Req)(msg))
			forwardHeaders(header, req.Header())
			resp, err := fn(ctx, req)
			if err != nil {
				return nil, err
			}
			return PRes(resp.Msg), nil
		},
	}
}

// serverStream adapts a generated server-streaming client method. The
// progress messages are dropped; the tool result is the final message.
func serverStream[Req, Res any, PReq protoPtr[Req], PRes protoPtr[Res]](
	fn func(context.Context, *connect.Request[Req]) (*connect.ServerStreamForClient[Res], error),
) rpc {
	return rpc{
		input: PReq(new(Req)).ProtoReflect().Descriptor(),
		call: func(ctx context.Context, header http.Header, args []byte) (proto.Message, error) {
			msg := PReq(new(Req))
			if err := decodeArgs(args, msg); err != nil {
				return nil, err
			}
			req := connect.NewRequest((*Req)(msg))
			forwardHeaders(header, req.Header())
			stream, err := fn(ctx, req)
			if err != nil {
				return nil, err
			}
			defer func() { _ = stream.Close() }()
			var last PRes
			for stream.Receive() {
				last = PRes(stream.Msg())
			}
			if err := stream.Err(); err != nil {
				return nil, err
			}
			if last == nil {
				last = PRes(new(Res))
			}
			return last, nil
		},
	}
}

func defaultLimit(limit *int32) {
	if *limit == 0 {
		*limit = defaultPageSize
	}
}

// tools returns the curated tool set. It deliberately leaves out RPCs that
// only make sense in the web UI (Stripe linking, settings and API keys, log
// streaming, CSV upload) and single-transaction writes that
// import_structured_transactions or bulk_edit_transactions cover.
func tools(c floatv1connect.LedgerServiceClient) []tool {
	return []tool{
		// ---- Reads ----
		{
			name:  "list_transactions",
			title: "List transactions",
			description: "List transactions matching hledger query tokens, oldest first, with fid, date, description, payee, status, tags and postings. " +
				"limit defaults to 100; page with offset (the response has total and has_next).",
			rpc: unary(c.ListTransactions, func(r *floatv1.ListTransactionsRequest) { defaultLimit(&r.Limit) }),
		},
		{
			name:        "get_account_register",
			title:       "Account register",
			description: "Register of one account: each transaction touching it with the running balance. limit defaults to 100.",
			rpc:         unary(c.GetAccountRegister, func(r *floatv1.GetAccountRegisterRequest) { defaultLimit(&r.Limit) }),
		},
		{
			name:        "get_balances",
			title:       "Account balances",
			description: "Account balances for hledger query tokens (e.g. [\"assets\", \"date:..2026-01-01\"]). Set value to \"now,USD\" for market value, depth to roll up sub-accounts, tree for parent totals.",
			rpc:         unary(c.GetBalances, nil),
		},
		{
			name:        "list_accounts",
			title:       "List accounts",
			description: "All accounts with their type and closed flag. Check this before writing so you reuse existing account names.",
			rpc:         unary(c.ListAccounts, nil),
		},
		{
			name:        "list_payees",
			title:       "List payees",
			description: "All payee names used in the ledger.",
			rpc:         unary(c.ListPayees, nil),
		},
		{
			name:        "list_tags",
			title:       "List tags",
			description: "All tag names used in the ledger.",
			rpc:         unary(c.ListTags, nil),
		},
		{
			name:        "get_income_statement_timeseries",
			title:       "Monthly income statement",
			description: "Monthly income and expenses per account between begin and end (YYYY-MM-DD; empty = all time / today), with net income per month.",
			rpc:         unary(c.GetIncomeStatementTimeseries, nil),
		},
		{
			name:        "get_net_worth_timeseries",
			title:       "Monthly net worth",
			description: "Month-end assets, liabilities and net worth between begin and end (YYYY-MM-DD).",
			rpc:         unary(c.GetNetWorthTimeseries, nil),
		},
		{
			name:        "get_portfolio_holdings",
			title:       "Portfolio holdings",
			description: "Investment holdings under an account prefix with quantity, latest price, market value, cost basis and unrealized gain.",
			rpc:         unary(c.GetPortfolioHoldings, nil),
		},
		{
			name:        "get_balance_assertion_status",
			title:       "Reconciliation status",
			description: "Asset and liability accounts ranked by how many transactions they have had since their last balance assertion, i.e. which most need reconciling.",
			rpc:         unary(c.GetBalanceAssertionStatus, nil),
		},
		{
			name:  "run_hledger_query",
			title: "Run hledger report",
			description: "Run a read-only hledger command against the ledger and get its text output, for reports the other tools don't cover, " +
				"e.g. \"bal expenses -M date:2026 --depth 2\", \"is -Q\", \"reg acct:visa date:lastmonth\". Omit the binary and -f. Add -O json for JSON output.",
			rpc: unary(c.RunHledgerQuery, nil),
		},
		{
			name:        "list_imports",
			title:       "List import batches",
			description: "Import batches (CSV, Stripe and structured imports) with batch id, date, source and transaction count.",
			rpc:         unary(c.ListImports, nil),
		},
		{
			name:        "get_imported_transactions",
			title:       "Transactions in an import batch",
			description: "Transactions written by one import batch. limit defaults to 100.",
			rpc:         unary(c.GetImportedTransactions, func(r *floatv1.GetImportedTransactionsRequest) { defaultLimit(&r.Limit) }),
		},
		{
			name:        "list_rules",
			title:       "List categorization rules",
			description: "float categorization rules: a regex on the description that sets payee, category account and tags, optionally marking matches reviewed.",
			rpc:         unary(c.ListRules, nil),
		},
		{
			name:        "preview_apply_rules",
			title:       "Preview applying rules",
			description: "Show what applying categorization rules (all, or rule_ids) to existing transactions matching query would change, without writing.",
			rpc:         unary(c.PreviewApplyRules, nil),
		},
		{
			name:        "list_snapshots",
			title:       "List snapshots",
			description: "Snapshots of the data directory, newest first; one is committed after every write. The first entry is the current state.",
			rpc:         unary(c.ListSnapshots, nil),
		},
		{
			name:        "get_snapshot_diff",
			title:       "Snapshot diff",
			description: "Unified diff of the files a snapshot changed relative to its parent.",
			rpc:         unary(c.GetSnapshotDiff, nil),
		},

		// ---- Writes ----
		{
			name:  "import_structured_transactions",
			title: "Import transactions",
			description: "Add transactions as one import batch. Validates the whole batch with hledger first (errors name the transaction index); dry_run previews without writing. " +
				"Transactions already in the ledger are skipped as duplicates, so re-sending is safe. apply_rules categorizes with float rules. " +
				"New transactions are Unmarked so the user can review them. Use this, not repeated single writes, for any new transactions.",
			effect:     additive,
			idempotent: true,
			rpc:        unary(c.ImportStructuredTransactions, nil),
		},
		{
			name:  "update_transaction",
			title: "Update a transaction",
			description: "Replace one transaction's description and postings (by fid). Postings are replaced wholesale; comment and tags are kept unless given or replace_comment/replace_tags is set. " +
				"Fetch it with list_transactions (query [\"code:FID\"]) first and send back what should stay.",
			effect:     destructive,
			idempotent: true,
			rpc:        unary(c.UpdateTransaction, nil),
		},
		{
			name:  "bulk_edit_transactions",
			title: "Bulk edit transactions",
			description: "Apply operations to many transactions by fid: mark reviewed/unreviewed, add/remove a tag, set/clear the payee, replace an account containing \"unknown\", or delete. " +
				"Each operations entry sets exactly one operation.",
			effect: destructive,
			rpc:    unary(c.BulkEditTransactions, nil),
		},
		{
			name:        "bulk_delete_transactions",
			title:       "Delete transactions",
			description: "Delete transactions by fid.",
			effect:      destructive,
			idempotent:  true,
			rpc:         serverStream(c.BulkDeleteTransactions),
		},
		{
			name:        "add_rule",
			title:       "Add categorization rules",
			description: "Add one or more categorization rules. Rules only affect future imports until applied with apply_rules.",
			effect:      additive,
			rpc:         unary(c.AddRule, nil),
		},
		{
			name:        "update_rule",
			title:       "Update a categorization rule",
			description: "Replace all fields of a rule by id (send unchanged fields back).",
			effect:      destructive,
			idempotent:  true,
			rpc:         unary(c.UpdateRule, nil),
		},
		{
			name:        "delete_rule",
			title:       "Delete a categorization rule",
			description: "Delete a rule by id.",
			effect:      destructive,
			idempotent:  true,
			rpc:         unary(c.DeleteRule, nil),
		},
		{
			name:        "apply_rules",
			title:       "Apply categorization rules",
			description: "Apply rules to existing transactions: pass the fids from preview_apply_rules with the same rule_ids and query.",
			effect:      additive,
			idempotent:  true,
			rpc:         serverStream(c.ApplyRules),
		},
		{
			name:  "restore_snapshot",
			title: "Restore a snapshot",
			description: "Reset the ledger, rules and templates to a snapshot (by hash from list_snapshots). " +
				"Every later snapshot is discarded with its changes, so this cannot be undone; confirm with the user first.",
			effect:     destructive,
			idempotent: true,
			rpc:        unary(c.RestoreSnapshot, nil),
		},
	}
}
