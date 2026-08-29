package cube

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// maxScale bounds the decimal places a commodity may declare. hledger permits
// arbitrary precision; int64 minor units do not. Eight places covers shares and
// the common crypto denominations while leaving ~92 billion units of headroom
// against int64 for a currency at that scale.
const maxScale = 8

// errAmountRange reports a decimal that cannot be held in int64 minor units at
// its commodity's scale.
var errAmountRange = errors.New("cube: amount out of int64 range")

// rawAmount is a decimal captured at whatever precision the source string
// carried, before it is rescaled to its commodity's common scale.
type rawAmount struct {
	mantissa int64
	scale    int
}

// parseDecimal splits an exact decimal string into an integer mantissa and the
// number of decimal places it carried: "-12.340" becomes (-12340, 3).
//
// Group separators are tolerated because hledger renders amounts in the
// commodity's declared display style, which may include them. An empty or
// whitespace-only string parses as zero, matching the empty cells a monthly
// balance report emits for periods an account did not exist in.
func parseDecimal(s string) (rawAmount, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return rawAmount{}, nil
	}

	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}
	if s == "" {
		return rawAmount{}, fmt.Errorf("cube: malformed decimal %q", s)
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if hasFrac && len(fracPart) > maxScale {
		// Trailing zeros beyond maxScale carry no value; anything else is a
		// precision we cannot represent and must not silently truncate.
		trimmed := strings.TrimRight(fracPart, "0")
		if len(trimmed) > maxScale {
			return rawAmount{}, fmt.Errorf("cube: %q has %d decimal places, limit is %d", s, len(trimmed), maxScale)
		}
		fracPart = fracPart[:maxScale]
	}

	var mant int64
	for _, digits := range []string{intPart, fracPart} {
		for i := 0; i < len(digits); i++ {
			d := digits[i]
			if d < '0' || d > '9' {
				return rawAmount{}, fmt.Errorf("cube: malformed decimal %q", s)
			}
			if mant > (math.MaxInt64-int64(d-'0'))/10 {
				return rawAmount{}, fmt.Errorf("%w: %q", errAmountRange, s)
			}
			mant = mant*10 + int64(d-'0')
		}
	}
	if neg {
		mant = -mant
	}
	return rawAmount{mantissa: mant, scale: len(fracPart)}, nil
}

// rescale converts a raw amount to the given scale. Scaling up multiplies;
// scaling down is rejected rather than rounded, since the caller always
// rescales to the maximum scale observed for the commodity and so should never
// lose digits.
func rescale(a rawAmount, to int) (int64, error) {
	switch {
	case a.scale == to:
		return a.mantissa, nil
	case a.scale > to:
		return 0, fmt.Errorf("cube: refusing to round %d at scale %d down to scale %d", a.mantissa, a.scale, to)
	}
	out := a.mantissa
	for i := a.scale; i < to; i++ {
		if out > math.MaxInt64/10 || out < math.MinInt64/10 {
			return 0, fmt.Errorf("%w: rescaling %d from %d to %d places", errAmountRange, a.mantissa, a.scale, to)
		}
		out *= 10
	}
	return out, nil
}
