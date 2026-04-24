package alphavantage

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"
)

const baseURL = "https://www.alphavantage.co/query"

// WeeklyPrice holds one week's closing price for a commodity.
type WeeklyPrice struct {
	Date  string // "YYYY-MM-DD" — week-ending date
	Close string // e.g. "188.63"
}

// Client fetches market data from Alpha Vantage.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchWeeklyPrices returns weekly closing prices for symbol in [startDate, endDate] inclusive.
// startDate and endDate are "YYYY-MM-DD". Results are sorted ascending by date.
func (c *Client) FetchWeeklyPrices(ctx context.Context, symbol, startDate, endDate string) ([]WeeklyPrice, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: invalid start_date %q: %w", startDate, err)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: invalid end_date %q: %w", endDate, err)
	}

	url := fmt.Sprintf("%s?function=TIME_SERIES_WEEKLY&symbol=%s&apikey=%s", baseURL, symbol, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alphavantage: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alphavantage: unexpected status %d", resp.StatusCode)
	}

	var body struct {
		WeeklyTimeSeries map[string]struct {
			Close string `json:"4. close"`
		} `json:"Weekly Time Series"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("alphavantage: decode response: %w", err)
	}
	if len(body.WeeklyTimeSeries) == 0 {
		return nil, fmt.Errorf("alphavantage: no weekly data returned for %q (invalid symbol or rate limit exceeded)", symbol)
	}

	var results []WeeklyPrice
	for dateStr, entry := range body.WeeklyTimeSeries {
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if (d.Equal(start) || d.After(start)) && (d.Equal(end) || d.Before(end)) {
			results = append(results, WeeklyPrice{Date: dateStr, Close: entry.Close})
		}
	}

	slices.SortFunc(results, func(a, b WeeklyPrice) int {
		return cmp.Compare(a.Date, b.Date)
	})

	return results, nil
}
