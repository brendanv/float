# internal/server/mcpserver

MCP (Model Context Protocol) endpoint for agents, mounted by `cmd/floatd` at `/mcp`. Built on `github.com/modelcontextprotocol/go-sdk`.

## Shape

- **Stateless streamable HTTP with JSON responses** (`Stateless`, `JSONResponse`): every POST stands alone, there are no session IDs or SSE streams, and it works behind ordinary reverse proxies. Server→client requests (sampling, elicitation) and progress notifications are therefore unavailable.
- **Tools are thin RPC adapters** (`tools.go`). Each `tool` names one `LedgerService` method of the generated client via `unary(...)` or `serverStream(...)`. Arguments are the request message in protojson (proto field names, unknown fields rejected). The result is the response message in protojson with `UseProtoNames`; zero values are omitted. For a server-streaming RPC it is the **last** message, with progress messages dropped. Errors come back as tool results with `IsError` (`<connect code>: <message>`, or `invalid arguments: ...`), not as protocol errors. Results over `maxResultBytes` become an error asking the agent to narrow the request. The exception is `import_structured_transactions` (`import_summary.go`): it returns a compact summary indexed by request position (counts, batch ID, fids, duplicate indices, what rules changed) instead of echoing every transaction, so large imports don't flood agent context or hit the cap after a successful write.
- **Few tools, on purpose.** There are 10: list_transactions, run_hledger_query, import_structured_transactions, update_transaction, bulk_edit_transactions, bulk_delete_transactions, and the rules tools list_rules, add_rule, preview_apply_rules, apply_rules. Every report-shaped read (balances, income statement, net worth, registers, account/payee/tag lists) goes through `run_hledger_query` instead of getting a tool of its own, because each tool definition costs agent context on every turn. `list_transactions` defaults `limit` to 100, because 0 means unlimited at the RPC level.
- **Calls go through Connect, not the handler directly.** `InProcess` (`inprocess.go`) serves the interceptor-wrapped service handler over `net.Pipe` connections, and tools call it with a normal generated client. Logging, the auth interceptor, validation and streaming then behave exactly as for external clients. The caller's `Authorization` header is forwarded on each call.
- **Auth** is applied outside this package: floatd wraps the handler in `auth.RequireBearer` (bearer only, no cookie, so a browser page can't use the endpoint with ambient credentials). With auth enabled, `NewHandler`'s `authEnabled` turns off the SDK's DNS-rebinding check, which would otherwise reject a same-host reverse proxy; the bearer already defeats rebinding.

## Schemas and docs

- `schema.go` generates each tool's input JSON Schema from the request message descriptor: nested messages are inlined, maps become objects, oneofs are noted in descriptions, and recursion is guarded.
- Field and message descriptions come from `.proto` comments. protoc-gen-go strips source info, so they are read from `ledger.binpb`, a `buf build --as-file-descriptor-set` image embedded in the binary. `mise run proto-gen` regenerates it and `mise run proto-check` fails when it is stale. **Proto comments are agent-facing documentation; keep them accurate.**
- `instructions.md` is embedded as the server's `instructions` (sent at initialize). It covers the data model, hledger query syntax, workflows (dry-run then import, categorizing with rules, reports) and the undo story. Update it when tool behavior changes.

## Adding or changing a tool

1. Add an entry to `tools()` with a snake_case name, a title, and an agent-oriented description saying when to use the tool and how it relates to others.
2. Set `readOnly`, `destructive` (can delete or overwrite) and `idempotent` (repeating the call changes nothing more) honestly. They map to MCP annotations, which clients use to decide when to ask the user.
3. Add a tool only for something `run_hledger_query` and the existing tools can't do. Leave out UI-only RPCs (settings, API keys, Stripe, snapshots, templates, log streams, CSV uploads), and prefer `import_structured_transactions` over single-transaction writes.
4. Extend `server_test.go`, which runs real hledger + gitsnap behind auth through the SDK client.
