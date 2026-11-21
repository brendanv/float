// Package mcpserver exposes a curated set of LedgerService RPCs as Model
// Context Protocol tools, served by floatd at /mcp.
//
// Tools are thin adapters: arguments are the RPC's request message in
// protojson (proto field names), the result is the response message in
// protojson, and input schemas are generated from the proto descriptors with
// the .proto comments as descriptions. Calls go through a regular Connect
// client to the in-process LedgerService handler, so auth, logging,
// validation and txlock behave exactly as for any other API client.
package mcpserver

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/brendanv/float/gen/float/v1/floatv1connect"
)

// Path is where floatd mounts the MCP endpoint.
const Path = "/mcp"

// maxResultBytes caps a tool result. Larger results are replaced by an error
// asking the agent to narrow the request, rather than flooding its context.
const maxResultBytes = 256 << 10

//go:embed instructions.md
var instructions string

// NewHandler returns the MCP endpoint, whose tools call client. It is
// stateless (every POST is self-contained) and answers with plain JSON, so it
// works behind ordinary reverse proxies. It does not authenticate: wrap it in
// auth.RequireBearer. Tool calls forward the request's Authorization header
// to the Connect API, which checks it again.
//
// authEnabled turns off the SDK's DNS-rebinding protection, which rejects
// requests to a loopback address with a non-loopback Host header (a reverse
// proxy on the same machine). With auth on, every request needs the bearer,
// which a rebinding page cannot supply.
func NewHandler(client floatv1connect.LedgerServiceClient, authEnabled bool) (http.Handler, error) {
	d, err := loadDocs(ledgerDescriptor)
	if err != nil {
		return nil, fmt.Errorf("mcp: load proto docs: %w", err)
	}
	s := mcp.NewServer(
		&mcp.Implementation{Name: "float", Title: "float personal finance ledger", Version: "1"},
		&mcp.ServerOptions{Instructions: instructions},
	)
	for _, t := range tools(client) {
		s.AddTool(t.mcpTool(d), t.handle)
	}
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{
			Stateless:                  true,
			JSONResponse:               true,
			DisableLocalhostProtection: authEnabled,
			MaxRequestBodyBytes:        32 << 20,
		},
	), nil
}

func (t tool) mcpTool(d docs) *mcp.Tool {
	ann := &mcp.ToolAnnotations{
		Title:          t.title,
		ReadOnlyHint:   t.readOnly,
		IdempotentHint: t.readOnly || t.idempotent,
		OpenWorldHint:  new(false),
	}
	if !t.readOnly {
		ann.DestructiveHint = new(t.destructive)
	}
	schema := d.inputSchema(t.rpc.input)
	delete(schema, "description") // the tool description covers the request message
	return &mcp.Tool{
		Name:        t.name,
		Title:       t.title,
		Description: t.description,
		InputSchema: schema,
		Annotations: ann,
	}
}

func (t tool) handle(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var header http.Header
	if req.Extra != nil {
		header = req.Extra.Header
	}
	res, err := t.rpc.call(ctx, header, req.Params.Arguments)
	if err != nil {
		slog.DebugContext(ctx, "mcp tool call failed", "tool", t.name, "error", err)
		return errorResult(err), nil
	}
	out, err := marshalResult(res)
	if err != nil {
		return nil, fmt.Errorf("marshal %s result: %w", t.name, err)
	}
	if len(out) > maxResultBytes {
		return errorResult(fmt.Errorf(
			"result too large (%d KiB, max %d KiB): narrow the query, set a smaller limit, or page with offset",
			len(out)>>10, maxResultBytes>>10)), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, nil
}

func marshalResult(res any) ([]byte, error) {
	if msg, ok := res.(proto.Message); ok {
		return protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	}
	return json.Marshal(res)
}

// errorResult reports a failed call as a tool error (visible to the model)
// rather than a protocol error.
func errorResult(err error) *mcp.CallToolResult {
	text := err.Error()
	if ce := new(connect.Error); errors.As(err, &ce) {
		text = fmt.Sprintf("%s: %s", ce.Code(), ce.Message())
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func decodeArgs(args []byte, msg proto.Message) error {
	if len(args) == 0 || strings.TrimSpace(string(args)) == "null" {
		return nil
	}
	if err := protojson.Unmarshal(args, msg); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

// forwardHeaders copies the caller's credential onto the outgoing Connect
// request.
func forwardHeaders(from, to http.Header) {
	if v := from.Get("Authorization"); v != "" {
		to.Set("Authorization", v)
	}
}
