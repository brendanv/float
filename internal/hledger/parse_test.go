package hledger

import "testing"

func TestParseLotAccount(t *testing.T) {
	tests := []struct {
		name      string
		full      string
		wantBase  string
		wantDate  string
		wantLabel string
		wantOK    bool
	}{
		{name: "date and cost", full: "assets:brokerage:{2026-01-15, $50}", wantBase: "assets:brokerage", wantDate: "2026-01-15", wantOK: true},
		{name: "label", full: `assets:brokerage:{2026-01-16, "mine", $40}`, wantBase: "assets:brokerage", wantDate: "2026-01-16", wantLabel: "mine", wantOK: true},
		{name: "cost with comma", full: "assets:brokerage:{2026-01-25, $1,234.567}", wantBase: "assets:brokerage", wantDate: "2026-01-25", wantOK: true},
		{name: "label with comma", full: `assets:brokerage:{2026-01-16, "a, b", $40}`, wantBase: "assets:brokerage", wantDate: "2026-01-16", wantLabel: "a, b", wantOK: true},
		{name: "date only", full: "assets:brokerage:{2026-01-15}", wantBase: "assets:brokerage", wantDate: "2026-01-15", wantOK: true},
		{name: "date and label only", full: `assets:brokerage:{2026-01-15, "x"}`, wantBase: "assets:brokerage", wantDate: "2026-01-15", wantLabel: "x", wantOK: true},
		{name: "nested base", full: "assets:us:brokerage:{2026-01-15, $50}", wantBase: "assets:us:brokerage", wantDate: "2026-01-15", wantOK: true},
		{name: "plain account", full: "assets:brokerage"},
		{name: "empty", full: ""},
		{name: "no date", full: "assets:brokerage:{$50}"},
		{name: "bad date", full: "assets:brokerage:{2026-1-15, $50}"},
		{name: "unterminated", full: "assets:brokerage:{2026-01-15, $50"},
		{name: "no base", full: ":{2026-01-15, $50}"},
		{name: "garbage after date", full: "assets:brokerage:{2026-01-15x}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, date, label, ok := parseLotAccount(tt.full)
			if ok != tt.wantOK || base != tt.wantBase || date != tt.wantDate || label != tt.wantLabel {
				t.Errorf("parseLotAccount(%q) = (%q, %q, %q, %v), want (%q, %q, %q, %v)",
					tt.full, base, date, label, ok, tt.wantBase, tt.wantDate, tt.wantLabel, tt.wantOK)
			}
		})
	}
}
