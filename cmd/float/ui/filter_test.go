package ui

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"

	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	tea "github.com/charmbracelet/bubbletea"
)

func newFilterClient() floatv1connect.LedgerServiceClient {
	return &mockLedgerClient{
		listTransactionsFn: func(_ context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
			return connect.NewResponse(&floatv1.ListTransactionsResponse{
				Transactions: nil,
			}), nil
		},
	}
}

func TestFilterInput_ActivateDeactivate(t *testing.T) {
	f := NewFilterInput(newFilterClient())

	if f.Active() {
		t.Fatal("expected inactive initially")
	}

	f.Activate()
	if !f.Active() {
		t.Fatal("expected active after Activate()")
	}

	f.Deactivate()
	if f.Active() {
		t.Fatal("expected inactive after Deactivate()")
	}
}

func TestFilterInput_Query_Empty(t *testing.T) {
	f := NewFilterInput(newFilterClient())
	if f.Query() != nil {
		t.Errorf("expected nil query for empty input, got %v", f.Query())
	}
}

func TestFilterInput_Deactivate_ReturnsNilQuery(t *testing.T) {
	var gotQuery []string
	client := &mockLedgerClient{
		listTransactionsFn: func(_ context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
			gotQuery = req.Msg.Query
			return connect.NewResponse(&floatv1.ListTransactionsResponse{}), nil
		},
	}
	f := NewFilterInput(client)
	f.Activate()
	cmd := f.Deactivate()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Deactivate")
	}
	cmd()
	if gotQuery != nil {
		t.Errorf("expected nil query, got %v", gotQuery)
	}
}

func TestFilterInput_Enter_FetchesWithQuery(t *testing.T) {
	var gotQuery []string
	client := &mockLedgerClient{
		listTransactionsFn: func(_ context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
			gotQuery = req.Msg.Query
			return connect.NewResponse(&floatv1.ListTransactionsResponse{}), nil
		},
	}
	f := NewFilterInput(client)
	f.Activate()

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("expenses food")})
	f, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from Enter")
	}
	cmd()
	if len(gotQuery) != 2 || gotQuery[0] != "expenses" || gotQuery[1] != "food" {
		t.Errorf("expected [expenses food], got %v", gotQuery)
	}
}

func TestFilterInput_Esc_FetchesNilQuery(t *testing.T) {
	var gotQuery []string
	called := false
	client := &mockLedgerClient{
		listTransactionsFn: func(_ context.Context, req *connect.Request[floatv1.ListTransactionsRequest]) (*connect.Response[floatv1.ListTransactionsResponse], error) {
			called = true
			gotQuery = req.Msg.Query
			return connect.NewResponse(&floatv1.ListTransactionsResponse{}), nil
		},
	}
	f := NewFilterInput(client)
	f.Activate()
	f, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected cmd from Esc")
	}
	cmd()
	if !called {
		t.Fatal("expected fetch to be called")
	}
	if gotQuery != nil {
		t.Errorf("expected nil query, got %v", gotQuery)
	}
	if f.Active() {
		t.Error("expected filter to be inactive after Esc")
	}
}

func TestFilterInput_View_InactiveEmpty(t *testing.T) {
	f := NewFilterInput(newFilterClient())
	if f.View() != "" {
		t.Errorf("expected empty view when inactive, got %q", f.View())
	}
}

func TestFilterInput_View_ActiveShowsPrompt(t *testing.T) {
	f := NewFilterInput(newFilterClient())
	f.Activate()
	view := f.View()
	if view == "" {
		t.Fatal("expected non-empty view when active")
	}
	if len(view) < 2 || view[:2] != "/ " {
		t.Errorf("expected view to start with '/ ', got %q", view)
	}
}
