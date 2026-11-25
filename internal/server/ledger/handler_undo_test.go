package ledger_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
)

// undoClient serves a handler (with snapshots and the undo interceptor) over
// HTTP and returns a client for it, so trailers are exercised for real.
func undoClient(t *testing.T, seed int64) (floatv1connect.LedgerServiceClient, string) {
	t.Helper()
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: seed, NumTxns: 4, WithFIDs: true})
	h, snap := mustRealHandlerWithSnap(t, dir)
	if err := snap.Commit(t.Context(), "seed"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(h, connect.WithInterceptors(serverledger.NewUndoInterceptor()))
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL), dir
}

func listFids(t *testing.T, c floatv1connect.LedgerServiceClient) []string {
	t.Helper()
	resp, err := c.ListTransactions(t.Context(), connect.NewRequest(&floatv1.ListTransactionsRequest{}))
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	var fids []string
	for _, tx := range resp.Msg.Transactions {
		fids = append(fids, tx.Fid)
	}
	if len(fids) < 2 {
		t.Fatalf("fixture has %d transactions, want >= 2", len(fids))
	}
	return fids
}

func deleteTx(t *testing.T, c floatv1connect.LedgerServiceClient, fid string) string {
	t.Helper()
	resp, err := c.DeleteTransaction(t.Context(), connect.NewRequest(&floatv1.DeleteTransactionRequest{Fid: fid}))
	if err != nil {
		t.Fatalf("DeleteTransaction: %v", err)
	}
	snapshot := resp.Trailer().Get(serverledger.SnapshotTrailer)
	if len(snapshot) != 40 {
		t.Fatalf("DeleteTransaction trailer %s = %q, want a commit hash", serverledger.SnapshotTrailer, snapshot)
	}
	return snapshot
}

func TestUndoSnapshot_UndoAndRedo(t *testing.T) {
	c, dir := undoClient(t, 800)
	fid := listFids(t, c)[0]
	snapshot := deleteTx(t, c, fid)
	if strings.Contains(journalText(t, dir), "("+fid+")") {
		t.Fatal("transaction not deleted")
	}

	undo, err := c.UndoSnapshot(t.Context(), connect.NewRequest(&floatv1.UndoSnapshotRequest{Hash: snapshot}))
	if err != nil {
		t.Fatalf("UndoSnapshot: %v", err)
	}
	if !strings.Contains(journalText(t, dir), "("+fid+")") {
		t.Error("undo did not bring the transaction back")
	}
	if !strings.HasPrefix(undo.Msg.Message, "undo: delete transaction "+fid) || len(undo.Msg.Snapshot) != 40 {
		t.Errorf("undo response = %+v", undo.Msg)
	}
	// UndoSnapshot reports its snapshot in the body, not the trailer.
	if got := undo.Trailer().Get(serverledger.SnapshotTrailer); got != "" {
		t.Errorf("UndoSnapshot set trailer %q", got)
	}

	redo, err := c.UndoSnapshot(t.Context(), connect.NewRequest(&floatv1.UndoSnapshotRequest{Hash: undo.Msg.Snapshot}))
	if err != nil {
		t.Fatalf("redo: %v", err)
	}
	if strings.Contains(journalText(t, dir), "("+fid+")") {
		t.Error("redo did not delete the transaction again")
	}
	if !strings.HasPrefix(redo.Msg.Message, "redo: delete transaction "+fid) {
		t.Errorf("redo message = %q", redo.Msg.Message)
	}
}

func TestUndoSnapshot_StaleIsRejected(t *testing.T) {
	c, dir := undoClient(t, 801)
	fids := listFids(t, c)
	first := deleteTx(t, c, fids[0])
	deleteTx(t, c, fids[1])
	before := journalText(t, dir)

	_, err := c.UndoSnapshot(t.Context(), connect.NewRequest(&floatv1.UndoSnapshotRequest{Hash: first}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("undo of an older snapshot: code %v (err %v), want FailedPrecondition", connect.CodeOf(err), err)
	}
	if journalText(t, dir) != before {
		t.Error("a rejected undo changed the journal")
	}
}

func TestUndoSnapshot_BadRequests(t *testing.T) {
	c, _ := undoClient(t, 802)
	for _, hash := range []string{"", "deadbeef", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"} {
		_, err := c.UndoSnapshot(t.Context(), connect.NewRequest(&floatv1.UndoSnapshotRequest{Hash: hash}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("hash %q: code %v (err %v), want InvalidArgument", hash, connect.CodeOf(err), err)
		}
	}
}

func TestUndoTrailer_Streaming(t *testing.T) {
	c, dir := undoClient(t, 803)
	fids := listFids(t, c)[:2]
	stream, err := c.BulkDeleteTransactions(t.Context(), connect.NewRequest(&floatv1.BulkDeleteTransactionsRequest{Fids: fids}))
	if err != nil {
		t.Fatalf("BulkDeleteTransactions: %v", err)
	}
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	snapshot := stream.ResponseTrailer().Get(serverledger.SnapshotTrailer)
	if len(snapshot) != 40 {
		t.Fatalf("streaming trailer = %q, want a commit hash", snapshot)
	}
	if _, err := c.UndoSnapshot(t.Context(), connect.NewRequest(&floatv1.UndoSnapshotRequest{Hash: snapshot})); err != nil {
		t.Fatalf("UndoSnapshot: %v", err)
	}
	for _, fid := range fids {
		if !strings.Contains(journalText(t, dir), "("+fid+")") {
			t.Errorf("transaction %s not restored by undo", fid)
		}
	}
}

func TestUndoTrailer_OnlyForUndoableCalls(t *testing.T) {
	c, _ := undoClient(t, 804)
	// AddTransaction is not on the undoable list.
	add, err := c.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
		Description: "Undo trailer test",
		Date:        "2026-01-15",
		Postings: []*floatv1.PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "1.00"},
			{Account: "assets:checking"},
		},
	}))
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}
	if got := add.Trailer().Get(serverledger.SnapshotTrailer); got != "" {
		t.Errorf("AddTransaction trailer = %q, want none", got)
	}
	// A failed undoable call gets no snapshot.
	_, err = c.DeleteTransaction(t.Context(), connect.NewRequest(&floatv1.DeleteTransactionRequest{Fid: "00000000"}))
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected deleting a missing transaction to fail with a Connect error, got %v", err)
	}
	if got := ce.Meta().Get(serverledger.SnapshotTrailer); got != "" {
		t.Errorf("failed call carried snapshot %q", got)
	}
}
