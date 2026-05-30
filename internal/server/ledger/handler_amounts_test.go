package ledger

import (
	"testing"

	"github.com/brendanv/float/internal/hledger"
)

func TestCollapseAmountsByCommodityExact(t *testing.T) {
	amounts := []hledger.Amount{
		amount("AAPL", 10, 0),
		amount("AAPL", 5, 0),
		amount("USD", 184230, 2),
	}

	got := collapseAmountsByCommodityExact(amounts)
	if len(got) != 2 {
		t.Fatalf("collapsed amount count = %d, want 2", len(got))
	}
	if got[0].Commodity != "AAPL" || got[0].Quantity.DecimalMantissa != 15 || got[0].Quantity.DecimalPlaces != 0 || got[0].Quantity.FloatingPoint != 15 {
		t.Errorf("first collapsed amount = %+v, want 15 AAPL", got[0])
	}
	if got[1].Commodity != "USD" || got[1].Quantity.DecimalMantissa != 184230 || got[1].Quantity.DecimalPlaces != 2 || got[1].Quantity.FloatingPoint != 1842.30 {
		t.Errorf("second collapsed amount = %+v, want 1842.30 USD", got[1])
	}
}

func TestCollapseAmountsByCommodityExactDropsExactZero(t *testing.T) {
	amounts := []hledger.Amount{
		amount("FUND", 1, 1),  // 0.1
		amount("FUND", 2, 1),  // 0.2
		amount("FUND", -3, 1), // -0.3
	}

	if got := collapseAmountsByCommodityExact(amounts); len(got) != 0 {
		t.Fatalf("collapsed amounts = %+v, want exact-zero commodity omitted", got)
	}
}

func amount(commodity string, mantissa int64, places int) hledger.Amount {
	q := exactQty{mantissa: mantissa, scale: places}
	return hledger.Amount{
		Commodity: commodity,
		Quantity: hledger.AmountQuantity{
			DecimalMantissa: mantissa,
			DecimalPlaces:   places,
			FloatingPoint:   exactQtyFloat(q),
		},
	}
}
