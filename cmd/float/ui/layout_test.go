package ui

import "testing"

func TestCalcLayout(t *testing.T) {
	tests := []struct {
		name        string
		w, h        int
		wantLeft    int
		wantRight   int
		wantContent int
	}{
		{"standard 80x24", 80, 24, 25, 55, 22},
		{"wide 120x40", 120, 40, 36, 84, 38},
		{"very wide 200x50", 200, 50, 45, 155, 48},
		{"narrow 60x15", 60, 15, 25, 35, 13},
		{"tiny below min width", 40, 10, 25, 15, 8},
		{"exact min width", 84, 24, 25, 59, 22},
		{"exact max width", 150, 24, 45, 105, 22},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalcLayout(tc.w, tc.h, 1)
			if got.LeftWidth != tc.wantLeft {
				t.Errorf("LeftWidth = %d, want %d", got.LeftWidth, tc.wantLeft)
			}
			if got.RightWidth != tc.wantRight {
				t.Errorf("RightWidth = %d, want %d", got.RightWidth, tc.wantRight)
			}
			if got.ContentHeight != tc.wantContent {
				t.Errorf("ContentHeight = %d, want %d", got.ContentHeight, tc.wantContent)
			}
			if got.LeftWidth+got.RightWidth != tc.w {
				t.Errorf("LeftWidth+RightWidth = %d, want %d (= w)", got.LeftWidth+got.RightWidth, tc.w)
			}
		})
	}
}
