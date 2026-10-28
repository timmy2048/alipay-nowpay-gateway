// Package money is the pricing and commission arithmetic for seat buyouts
// and customer receipts. Amounts are integer fen (0.01 CNY). Rates are
// basis points: 100 bps = 1.00%, 10000 bps = 100%.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	BillingOneTime  = "ONE_TIME"
	BillingDuration = "ONE_TIME_DURATION"
	BillingQuantity = "QUANTITY"
	QuotaCount      = "COUNT"
	QuotaPoint      = "POINT"
)

var (
	ErrAmount       = errors.New("金额无效")
	ErrRate         = errors.New("费率无效")
	ErrOverflow     = errors.New("金额溢出")
	ErrMode         = errors.New("未知计费模式")
	ErrModeReserved = errors.New("该计费模式已预留，阶段一只开放永久买断 ONE_TIME")
)

// ParseYuan accepts a non-negative decimal yuan string with at most two
// fraction digits. "199.00", "199" and "0.1" are valid. The result is fen.
func ParseYuan(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		return 0, ErrAmount
	}
	whole, frac, ok := strings.Cut(s, ".")
	if strings.Contains(frac, ".") || !ok && strings.Contains(s, ".") {
		return 0, ErrAmount
	}
	if whole == "" || !digits(whole) {
		return 0, ErrAmount
	}
	if len(whole) > 1 && whole[0] == '0' {
		return 0, ErrAmount
	}
	if ok {
		if frac == "" || len(frac) > 2 || !digits(frac) {
			return 0, ErrAmount
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrAmount
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, ErrAmount
	}
	if w > (math.MaxInt64-f)/100 {
		return 0, ErrOverflow
	}
	return w*100 + f, nil
}

// FormatYuan renders fen as a two-decimal yuan string without a currency sign.
func FormatYuan(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// PercentToBPS parses a percent with at most two decimals into basis points.
// "2.5" is 250 bps. "100" is 10000. Values above 100% are rejected.
func PercentToBPS(s string) (int, error) {
	v, err := ParseYuan(s)
	if err != nil || v > 10000 {
		return 0, ErrRate
	}
	return int(v), nil
}

// MulBPS rounds amount * bps / 10000 to the nearest fen, half up.
func MulBPS(amountCents int64, bps int) (int64, error) {
	if amountCents < 0 || bps < 0 || bps > 10000 {
		return 0, ErrRate
	}
	if bps == 0 || amountCents == 0 {
		return 0, nil
	}
	if amountCents > (math.MaxInt64-5000)/int64(bps) {
		return 0, ErrOverflow
	}
	return (amountCents*int64(bps) + 5000) / 10000, nil
}

// ResolveSeatPrice picks the project override when set, otherwise the catalog
// price. Each new Alipay account is quoted again, so a later price change
// does not rewrite seats already purchased.
func ResolveSeatPrice(catalogCents int64, override *int64) (int64, error) {
	if override != nil {
		if *override < 0 {
			return 0, ErrAmount
		}
		return *override, nil
	}
	if catalogCents < 0 {
		return 0, ErrAmount
	}
	return catalogCents, nil
}

// SumCents adds independent seat snapshots. Buying a second account is a new
// line at the price then in force, not a quantity discount.
func SumCents(parts ...int64) (int64, error) {
	var total int64
	for _, p := range parts {
		if p < 0 {
			return 0, ErrAmount
		}
		if total > math.MaxInt64-p {
			return 0, ErrOverflow
		}
		total += p
	}
	return total, nil
}

// ValidatePhase1Buyout accepts only the permanent seat buyout in phase 1.
// Duration, quantity and points stay in the schema for a later phase.
func ValidatePhase1Buyout(mode string) error {
	switch mode {
	case BillingOneTime:
		return nil
	case BillingDuration, BillingQuantity:
		return fmt.Errorf("%w", ErrModeReserved)
	default:
		return fmt.Errorf("%w: %s", ErrMode, mode)
	}
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
