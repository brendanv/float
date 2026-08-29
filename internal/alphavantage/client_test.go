package alphavantage_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brendanv/float/internal/alphavantage"
)

// newTestServer returns an httptest.Server that responds with the given
// status and body for every request, and records the last request's query
// string into *lastQuery when non-nil.
func newTestServer(t *testing.T, status int, body string, lastQuery *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lastQuery != nil {
			*lastQuery = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const sampleWeeklyBody = `{
	"Weekly Time Series": {
		"2024-01-05": {"4. close": "100.00"},
		"2024-01-12": {"4. close": "101.50"},
		"2024-01-19": {"4. close": "99.75"},
		"2024-01-26": {"4. close": "102.25"}
	}
}`

func TestFetchWeeklyPrices_FiltersAndSortsByDate(t *testing.T) {
	var query string
	srv := newTestServer(t, http.StatusOK, sampleWeeklyBody, &query)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	prices, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2024-01-06", "2024-01-20")
	if err != nil {
		t.Fatalf("FetchWeeklyPrices: %v", err)
	}

	if len(prices) != 2 {
		t.Fatalf("expected 2 prices in range, got %d: %+v", len(prices), prices)
	}
	if prices[0].Date != "2024-01-12" || prices[1].Date != "2024-01-19" {
		t.Errorf("prices out of expected order: %+v", prices)
	}
	if prices[0].Date > prices[1].Date {
		t.Errorf("prices not sorted ascending: %+v", prices)
	}

	if query == "" {
		t.Fatal("expected request to reach test server")
	}
}

func TestFetchWeeklyPrices_InclusiveBoundaryDates(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, sampleWeeklyBody, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	prices, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2024-01-05", "2024-01-26")
	if err != nil {
		t.Fatalf("FetchWeeklyPrices: %v", err)
	}
	if len(prices) != 4 {
		t.Fatalf("expected all 4 boundary-inclusive prices, got %d: %+v", len(prices), prices)
	}
	if prices[0].Date != "2024-01-05" || prices[3].Date != "2024-01-26" {
		t.Errorf("boundary dates not included: %+v", prices)
	}
}

func TestFetchWeeklyPrices_ResultFieldsPopulated(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, sampleWeeklyBody, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	prices, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2024-01-05", "2024-01-05")
	if err != nil {
		t.Fatalf("FetchWeeklyPrices: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 price, got %d", len(prices))
	}
	got := prices[0]
	want := alphavantage.WeeklyPrice{Date: "2024-01-05", Close: "100.00", Currency: "USD"}
	if got != want {
		t.Errorf("price = %+v, want %+v", got, want)
	}
}

func TestFetchWeeklyPrices_NoResultsInRange(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, sampleWeeklyBody, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	prices, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2023-01-01", "2023-12-31")
	if err != nil {
		t.Fatalf("FetchWeeklyPrices: %v", err)
	}
	if len(prices) != 0 {
		t.Errorf("expected no prices in range, got %+v", prices)
	}
}

func TestFetchWeeklyPrices_InvalidDateArgs(t *testing.T) {
	cl := alphavantage.NewClient("test-key")

	tests := []struct {
		name  string
		start string
		end   string
	}{
		{"invalid start date", "not-a-date", "2024-01-01"},
		{"invalid end date", "2024-01-01", "not-a-date"},
		{"wrong format", "01/05/2024", "2024-01-20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", tt.start, tt.end)
			if err == nil {
				t.Fatal("expected error for invalid date, got nil")
			}
		})
	}
}

func TestFetchWeeklyPrices_NonOKStatus(t *testing.T) {
	srv := newTestServer(t, http.StatusInternalServerError, `{"error":"boom"}`, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	_, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
}

func TestFetchWeeklyPrices_EmptyTimeSeries(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, `{"Weekly Time Series": {}}`, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	_, err := cl.FetchWeeklyPrices(t.Context(), "BADSYM", "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error for empty weekly time series, got nil")
	}
}

func TestFetchWeeklyPrices_MissingTimeSeriesKey(t *testing.T) {
	// Alpha Vantage returns a body without "Weekly Time Series" on rate-limit
	// or invalid-symbol responses (e.g. a "Note" or "Error Message" field).
	srv := newTestServer(t, http.StatusOK, `{"Error Message": "Invalid API call"}`, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	_, err := cl.FetchWeeklyPrices(t.Context(), "BADSYM", "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error when time series key is absent, got nil")
	}
}

func TestFetchWeeklyPrices_MalformedJSON(t *testing.T) {
	srv := newTestServer(t, http.StatusOK, `{not valid json`, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	_, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error for malformed JSON response, got nil")
	}
}

func TestFetchWeeklyPrices_SkipsUnparseableDateKeys(t *testing.T) {
	body := `{
		"Weekly Time Series": {
			"2024-01-05": {"4. close": "100.00"},
			"not-a-date": {"4. close": "999.00"}
		}
	}`
	srv := newTestServer(t, http.StatusOK, body, nil)
	defer srv.Close()

	cl := alphavantage.NewClient("test-key", alphavantage.WithBaseURL(srv.URL))

	prices, err := cl.FetchWeeklyPrices(t.Context(), "AAPL", "2000-01-01", "2030-01-01")
	if err != nil {
		t.Fatalf("FetchWeeklyPrices: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected unparseable date entry to be skipped, got %+v", prices)
	}
	if prices[0].Date != "2024-01-05" {
		t.Errorf("unexpected surviving entry: %+v", prices[0])
	}
}
