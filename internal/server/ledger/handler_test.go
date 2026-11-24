package ledger_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	floatv1connect "github.com/brendanv/float/gen/float/v1/floatv1connect"
	"github.com/brendanv/float/internal/cache"
	"github.com/brendanv/float/internal/hledger"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/txlock"
)

// importTransactions calls the streaming ImportTransactions RPC via a test HTTP
// server and returns the final ImportTransactionsResult.
func importTransactions(t *testing.T, h *serverledger.Handler, req *floatv1.ImportTransactionsRequest) (*floatv1.ImportTransactionsResult, error) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(h)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)
	stream, err := client.ImportTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = stream.Close() })

	var result *floatv1.ImportTransactionsResult
	for stream.Receive() {
		if p, ok := stream.Msg().Payload.(*floatv1.ImportTransactionsResponse_Result); ok {
			result = p.Result
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// versionRunner returns a valid hledger version string for client construction.
func versionRunner(t *testing.T, data map[string][]byte) hledger.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		key := strings.Join(args, " ")
		for k, v := range data {
			if strings.Contains(key, k) {
				return v, nil, nil
			}
		}
		return []byte("[]"), nil, nil
	}
}

func mustHandler(t *testing.T, data map[string][]byte) *serverledger.Handler {
	t.Helper()
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", versionRunner(t, data))
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	return serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
}

const printJSON = `[
  {
    "tcode": "aa001100",
    "tcomment": "",
    "tdate": "2026-01-05",
    "tdate2": null,
    "tdescription": "PAYROLL DIRECT DEPOSIT",
    "tindex": 1,
    "tpostings": [
      {
        "paccount": "assets:checking",
        "pamount": [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 350000, "decimalPlaces": 2, "floatingPoint": 3500}}],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "preal": "RealPosting"
      },
      {
        "paccount": "income:salary",
        "pamount": [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": -350000, "decimalPlaces": 2, "floatingPoint": -3500}}],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "preal": "RealPosting"
      }
    ],
    "tprecedingcomment": "",
    "tstatus": "Unmarked",
    "ttags": [],
    "tsourcepos": [{"sourceName": "simple.journal", "sourceLine": 1, "sourceColumn": 1}, {"sourceName": "simple.journal", "sourceLine": 4, "sourceColumn": 1}]
  }
]`

const balJSON = `[[["assets:checking", "assets:checking", 0, [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 700000, "decimalPlaces": 2, "floatingPoint": 7000}}]], ["income:salary", "income:salary", 0, [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": -700000, "decimalPlaces": 2, "floatingPoint": -7000}}]]], [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 0, "decimalPlaces": 2, "floatingPoint": 0}}]]`

// bsTimeseriesJSON is a minimal hledger bs --monthly -O json fixture with 2 periods,
// Assets and Liabilities subreports, and net worth totals.
const bsTimeseriesJSON = `{
  "cbrDates": [
    [{"contents": "2026-01-01", "tag": "Exact"}, {"contents": "2026-02-01", "tag": "Exact"}],
    [{"contents": "2026-02-01", "tag": "Exact"}, {"contents": "2026-03-01", "tag": "Exact"}]
  ],
  "cbrSubreports": [
    ["Assets", {
      "prDates": [],
      "prRows": [],
      "prTotals": {
        "prrAmounts": [
          [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 333500, "decimalPlaces": 2, "floatingPoint": 3335}}],
          [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 682000, "decimalPlaces": 2, "floatingPoint": 6820}}]
        ],
        "prrAverage": [], "prrName": [], "prrTotal": []
      }
    }],
    ["Liabilities", {
      "prDates": [],
      "prRows": [],
      "prTotals": {"prrAmounts": [[], []], "prrAverage": [], "prrName": [], "prrTotal": []}
    }]
  ],
  "cbrTitle": "Balance Sheet",
  "cbrTotals": {
    "prrAmounts": [
      [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 333500, "decimalPlaces": 2, "floatingPoint": 3335}}],
      [{"acommodity": "$", "acost": null, "aquantity": {"decimalMantissa": 682000, "decimalPlaces": 2, "floatingPoint": 6820}}]
    ],
    "prrAverage": [], "prrName": [], "prrTotal": []
  }
}`

const accountsText = `assets:checking      ; type: A
income:salary        ; type: R
`

// mustRealHandler creates a handler backed by a real hledger client and data dir.
func mustRealHandler(t *testing.T, dir string) *serverledger.Handler {
	t.Helper()
	c, err := hledger.New("hledger", dir+"/main.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	lock := txlock.New(dir, c)
	return serverledger.NewHandler(c, lock, dir, "", nil, nil, nil, nil)
}

// errorRunner returns a runner that fails on every non-version call.
func errorRunner(t *testing.T) hledger.CommandRunner {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("hledger 1.99.6, linux-x86_64\n"), nil, nil
		}
		return nil, nil, errors.New("hledger failed")
	}
}

func mustHandlerWithCache(t *testing.T, runner hledger.CommandRunner) (*serverledger.Handler, *cache.Cache[any]) {
	t.Helper()
	c, err := hledger.NewWithRunner("hledger", "testdata/simple.journal", runner)
	if err != nil {
		t.Fatalf("NewWithRunner: %v", err)
	}
	ch := cache.New[any](func() uint64 { return 0 })
	return serverledger.NewHandler(c, nil, "", "", ch, nil, nil, nil), ch
}

const payeesText = "Acme Corp\nGrocery Store\n"

// aregJSON is a two-row fixture for `hledger areg assets:checking -O json`.
// Each row is the documented 6-element array:
//
//	[Transaction, Transaction, Bool, [string], [Amount], [Amount]].
const aregJSON = `[
  [
    {"tcode":"aa001100","tcomment":"","tdate":"2026-01-05","tdate2":null,"tdescription":"PAYROLL | monthly","tindex":1,"tpostings":[],"tprecedingcomment":"","tstatus":"Cleared","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    {"tcode":"aa001100","tcomment":"","tdate":"2026-01-05","tdate2":null,"tdescription":"PAYROLL | monthly","tindex":1,"tpostings":[],"tprecedingcomment":"","tstatus":"Cleared","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    false,
    ["income:salary"],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":350000,"decimalPlaces":2,"floatingPoint":3500}}]
  ],
  [
    {"tcode":"bb002200","tcomment":"","tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tindex":2,"tpostings":[],"tprecedingcomment":"","tstatus":"Unmarked","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    {"tcode":"bb002200","tcomment":"","tdate":"2026-01-15","tdate2":null,"tdescription":"AMAZON","tindex":2,"tpostings":[],"tprecedingcomment":"","tstatus":"Unmarked","ttags":[],"tsourcepos":[{"sourceName":"","sourceLine":0,"sourceColumn":0},{"sourceName":"","sourceLine":0,"sourceColumn":0}]},
    false,
    ["expenses:shopping"],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":-4500,"decimalPlaces":2,"floatingPoint":-45}}],
    [{"acommodity":"$","acost":null,"aquantity":{"decimalMantissa":345500,"decimalPlaces":2,"floatingPoint":3455}}]
  ]
]`

// bulkDeleteTransactions calls the streaming BulkDeleteTransactions RPC via a test
// HTTP server and returns the final result.
func bulkDeleteTransactions(t *testing.T, h *serverledger.Handler, req *floatv1.BulkDeleteTransactionsRequest) (*floatv1.BulkDeleteTransactionsResult, error) {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := floatv1connect.NewLedgerServiceHandler(h)
	mux.Handle(path, handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := floatv1connect.NewLedgerServiceClient(srv.Client(), srv.URL)
	stream, err := client.BulkDeleteTransactions(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = stream.Close() })

	var result *floatv1.BulkDeleteTransactionsResult
	for stream.Receive() {
		if p, ok := stream.Msg().Payload.(*floatv1.BulkDeleteTransactionsResponse_Result); ok {
			result = p.Result
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// printJSONWithCost has two postings: one with a UnitCost (@) and one with a
// TotalCost (@@). Used to verify toProtoAmount surfaces cost annotations.
const printJSONWithCost = `[
  {
    "tcode": "bb002200",
    "tcomment": "",
    "tdate": "2026-01-15",
    "tdate2": null,
    "tdescription": "Buy AAPL",
    "tindex": 1,
    "tpostings": [
      {
        "paccount": "assets:investments:aapl",
        "pamount": [{
          "acommodity": "AAPL",
          "acost": {"tag":"UnitCost","contents":{"acommodity":"USD","aquantity":{"decimalMantissa":17500,"decimalPlaces":2,"floatingPoint":175.00},"acost":null}},
          "aquantity": {"decimalMantissa": 10, "decimalPlaces": 0, "floatingPoint": 10}
        }],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "preal": "RealPosting"
      },
      {
        "paccount": "assets:investments:msft",
        "pamount": [{
          "acommodity": "MSFT",
          "acost": {"tag":"TotalCost","contents":{"acommodity":"USD","aquantity":{"decimalMantissa":200000,"decimalPlaces":2,"floatingPoint":2000.00},"acost":null}},
          "aquantity": {"decimalMantissa": 5, "decimalPlaces": 0, "floatingPoint": 5}
        }],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "preal": "RealPosting"
      },
      {
        "paccount": "assets:checking",
        "pamount": [{"acommodity":"USD","acost":null,"aquantity":{"decimalMantissa":-375000,"decimalPlaces":2,"floatingPoint":-3750.00}}],
        "pcomment": "",
        "pdate": null,
        "pdate2": null,
        "pstatus": "Unmarked",
        "ptags": [],
        "ptransaction_": "1",
        "preal": "RealPosting"
      }
    ],
    "tprecedingcomment": "",
    "tstatus": "Unmarked",
    "ttags": [],
    "tsourcepos": [{"sourceName": "simple.journal", "sourceLine": 1, "sourceColumn": 1}, {"sourceName": "simple.journal", "sourceLine": 5, "sourceColumn": 1}]
  }
]`

func mustLotsHandler(t *testing.T) *serverledger.Handler {
	t.Helper()
	c, err := hledger.New("hledger", "../../hledger/testdata/lots.journal")
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	return serverledger.NewHandler(c, nil, "", "", nil, nil, nil, nil)
}
