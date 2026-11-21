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

// defaultPageSize is the limit list_transactions applies when the caller
// leaves it at 0, which the RPC treats as "no limit". An unbounded
// transaction list can run to megabytes, far past what an agent can read.
const defaultPageSize = 100

// tool is one LedgerService RPC exposed over MCP.
type tool struct {
	name        string
	title       string
	description string
	readOnly    bool
	destructive bool // may delete or overwrite existing data
	idempotent  bool // repeating the call with the same arguments changes nothing more
	rpc         rpc
}

// rpc decodes tool arguments into a request message, calls the RPC with the
// given request headers and returns the response message.
type rpc struct {
	input protoreflect.MessageDescriptor
	call  func(ctx context.Context, header http.Header, args []byte) (any, error)
}

type protoPtr[T any] interface {
	*T
	proto.Message
}

// decodeRequest builds the Connect request for a tool call.
func decodeRequest[Req any, PReq protoPtr[Req]](header http.Header, args []byte) (*connect.Request[Req], error) {
	msg := PReq(new(Req))
	if err := decodeArgs(args, msg); err != nil {
		return nil, err
	}
	req := connect.NewRequest((*Req)(msg))
	forwardHeaders(header, req.Header())
	return req, nil
}

// unary adapts a generated unary client method.
func unary[Req, Res any, PReq protoPtr[Req], PRes protoPtr[Res]](
	fn func(context.Context, *connect.Request[Req]) (*connect.Response[Res], error),
) rpc {
	return rpc{
		input: PReq(new(Req)).ProtoReflect().Descriptor(),
		call: func(ctx context.Context, header http.Header, args []byte) (any, error) {
			req, err := decodeRequest[Req, PReq](header, args)
			if err != nil {
				return nil, err
			}
			resp, err := fn(ctx, req)
			if err != nil {
				return nil, err
			}
			return PRes(resp.Msg), nil
		},
	}
}

// serverStream adapts a generated server-streaming client method. Progress
// messages are dropped; the tool result is the final message.
func serverStream[Req, Res any, PReq protoPtr[Req], PRes protoPtr[Res]](
	fn func(context.Context, *connect.Request[Req]) (*connect.ServerStreamForClient[Res], error),
) rpc {
	return rpc{
		input: PReq(new(Req)).ProtoReflect().Descriptor(),
		call: func(ctx context.Context, header http.Header, args []byte) (any, error) {
			req, err := decodeRequest[Req, PReq](header, args)
			if err != nil {
				return nil, err
			}
			stream, err := fn(ctx, req)
			if err != nil {
				return nil, err
			}
			defer func() { _ = stream.Close() }()
			last := PRes(new(Res))
			for stream.Receive() {
				last = PRes(stream.Msg())
			}
			return last, stream.Err()
		},
	}
}

// listTransactions applies defaultPageSize to ListTransactions.
func listTransactions(c floatv1connect.LedgerServiceClient) rpc {
	return unary(func(ctx context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
		if req.Msg.Limit == 0 {
			req.Msg.Limit = defaultPageSize
		}
		return c.ListTransactions(ctx, req)
	})
}

// tools returns the tool set: find transactions, report on them, write them,
// and categorize them. Everything report-shaped (balances, income statement,
// net worth, registers, account/payee/tag lists) goes through
// run_hledger_query rather than a tool per report; UI-only RPCs (settings,
// Stripe, CSV upload, snapshots, templates) are left out.
func tools(c floatv1connect.LedgerServiceClient) []tool {
	return []tool{
		{
			name:  "list_transactions",
			title: "List transactions",
			description: "List transactions matching hledger query tokens, oldest first, with fid, date, description, payee, status, tags and postings. " +
				"limit defaults to 100; page with offset (the response has total and has_next).",
			readOnly: true,
			rpc:      listTransactions(c),
		},
		{
			name:  "run_hledger_query",
			title: "Run hledger report",
			description: "Run a read-only hledger command and get its text output. Use it for every report: " +
				"\"accounts\", \"payees\", \"bal expenses -M date:2026 --depth 2\", \"bal assets -V\", \"is -M\", \"bs\", \"reg acct:visa date:lastmonth\". " +
				"Omit the binary and -f. Add -O json for JSON output.",
			readOnly: true,
			rpc:      unary(c.RunHledgerQuery),
		},
		{
			name:  "import_structured_transactions",
			title: "Import transactions",
			description: "Add transactions as one import batch. Validates the whole batch with hledger first (errors name the transaction index); dry_run previews without writing. " +
				"Transactions already in the ledger are skipped as duplicates, so re-sending is safe. apply_rules categorizes with float rules. " +
				"New transactions are Unmarked so the user can review them. Use this for any new transactions, even one. " +
				"The result is a compact summary, not the transactions: counts, import_batch_id, fids (one per input, in order; \"\" for skipped duplicates), duplicates (input indices) and rule_matches (what a rule changed, by input index). " +
				"To see what was written, list_transactions with query [\"tag:float-import=<import_batch_id>\"].",
			idempotent: true,
			rpc:        importStructured(c),
		},
		{
			name:  "update_transaction",
			title: "Update a transaction",
			description: "Replace one transaction's description and postings (by fid), e.g. to recategorize it. Postings are replaced wholesale; comment and tags are kept unless given or replace_comment/replace_tags is set. " +
				"Fetch it with list_transactions (query [\"code:FID\"]) first and send back what should stay.",
			destructive: true,
			idempotent:  true,
			rpc:         unary(c.UpdateTransaction),
		},
		{
			name:  "bulk_edit_transactions",
			title: "Bulk edit transactions",
			description: "Apply operations to transactions by fid: mark reviewed/unreviewed, add/remove a tag, set/clear the payee, or replace an account containing \"unknown\". " +
				"Each operations entry sets exactly one operation (not delete: use bulk_delete_transactions).",
			destructive: true,
			rpc:         unary(c.BulkEditTransactions),
		},
		{
			name:        "bulk_delete_transactions",
			title:       "Delete transactions",
			description: "Delete transactions by fid, e.g. the fids an import returned, to undo it.",
			destructive: true,
			idempotent:  true,
			rpc:         serverStream(c.BulkDeleteTransactions),
		},
		{
			name:        "list_rules",
			title:       "List categorization rules",
			description: "float categorization rules: a regex on the description that sets payee, category account and tags, optionally marking matches reviewed.",
			readOnly:    true,
			rpc:         unary(c.ListRules),
		},
		{
			name:        "add_rule",
			title:       "Add categorization rules",
			description: "Add one or more categorization rules. They apply to future imports; use preview_apply_rules and apply_rules for existing transactions.",
			rpc:         unary(c.AddRule),
		},
		{
			name:        "preview_apply_rules",
			title:       "Preview applying rules",
			description: "Show what applying categorization rules (all, or rule_ids) to existing transactions matching query would change, without writing.",
			readOnly:    true,
			rpc:         unary(c.PreviewApplyRules),
		},
		{
			name:        "apply_rules",
			title:       "Apply categorization rules",
			description: "Apply rules to existing transactions: pass the fids from preview_apply_rules with the same rule_ids and query.",
			idempotent:  true,
			rpc:         serverStream(c.ApplyRules),
		},
	}
}
