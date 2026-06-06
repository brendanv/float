package ui

import "testing"

func TestCalcLayout(t *testing.T) {
	tests := []struct {
		name        string
		h           int
		wantContent int
	}{
		{"standard 24h", 24, 22},
		{"tall 40h", 40, 38},
		{"very tall 50h", 50, 48},
		{"short 15h", 15, 13},
		{"tiny 10h", 10, 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalcLayout(tc.h, 1)
			if got.ContentHeight != tc.wantContent {
				t.Errorf("ContentHeight = %d, want %d", got.ContentHeight, tc.wantContent)
			}
		})
	}
}
