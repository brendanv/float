package hledger

import "testing"

func TestAmountQuantityFormat(t *testing.T) {
	tests := []struct {
		mantissa int64
		places   int
		want     string
	}{
		{0, 0, "0"},
		{0, 2, "0.00"},
		{1, 0, "1"},
		{-1, 0, "-1"},
		{100, 2, "1.00"},
		{-100, 2, "-1.00"},
		{17500, 2, "175.00"},
		{-17500, 2, "-175.00"},
		{200000, 2, "2000.00"},
		{-375000, 2, "-3750.00"},
		{10, 0, "10"},
		{5, 2, "0.05"},
		{1, 2, "0.01"},
		{123456789, 4, "12345.6789"},
		{-123456789, 4, "-12345.6789"},
		// mantissa shorter than decimal places: leading zeros after decimal point
		{1, 4, "0.0001"},
		{12, 4, "0.0012"},
	}
	for _, tt := range tests {
		q := AmountQuantity{DecimalMantissa: tt.mantissa, DecimalPlaces: tt.places}
		if got := q.Format(); got != tt.want {
			t.Errorf("Format(%d, %d) = %q, want %q", tt.mantissa, tt.places, got, tt.want)
		}
	}
}
