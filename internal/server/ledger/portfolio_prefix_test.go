package ledger

import "testing"

func TestAccountMatchesPrefix(t *testing.T) {
	tests := []struct {
		account, prefix string
		want            bool
	}{
		{"assets:investments:aapl", "assets", true},
		{"Assets:Investments:AAPL", "assets", true},
		{"assets:investments:aapl", "Assets", true},
		{"assets", "assets", true},
		{"Assets", "assets", true},
		{"expenses:shopping", "assets", false},
		{"assetsxyz", "assets", false},
	}
	for _, tt := range tests {
		if got := accountMatchesPrefix(tt.account, tt.prefix); got != tt.want {
			t.Errorf("accountMatchesPrefix(%q, %q) = %v, want %v", tt.account, tt.prefix, got, tt.want)
		}
	}
}

func TestIsExcludedAccount(t *testing.T) {
	prefixes := []string{"assets:cash"}
	tests := []struct {
		account string
		want    bool
	}{
		{"assets:cash", true},
		{"Assets:Cash", true},
		{"assets:cash:wallet", true},
		{"Assets:Cash:Wallet", true},
		{"assets:checking", false},
	}
	for _, tt := range tests {
		if got := isExcludedAccount(tt.account, prefixes); got != tt.want {
			t.Errorf("isExcludedAccount(%q, %v) = %v, want %v", tt.account, prefixes, got, tt.want)
		}
	}
}
