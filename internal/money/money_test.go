package money

import "testing"

func TestParseYuan(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"199.00", 19900, true},
		{"199", 19900, true},
		{"0.1", 10, true},
		{"0.10", 10, true},
		{"0.01", 1, true},
		{"0", 0, true},
		{"0.00", 0, true},
		{" 29.9 ", 2990, true},
		{"1.001", 0, false},
		{"-1", 0, false},
		{"00.10", 0, false},
		{".5", 0, false},
		{"1.", 0, false},
		{"", 0, false},
		{"+1", 0, false},
		{"1.2.3", 0, false},
	}
	for _, tc := range cases {
		got, err := ParseYuan(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ParseYuan(%q)=%d,%v want %d", tc.in, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Fatalf("ParseYuan(%q) unexpectedly ok (%d)", tc.in, got)
		}
	}
}

func TestFormatYuan(t *testing.T) {
	if got := FormatYuan(19900); got != "199.00" {
		t.Fatal(got)
	}
	if got := FormatYuan(10); got != "0.10" {
		t.Fatal(got)
	}
}

func TestPercentToBPS(t *testing.T) {
	cases := map[string]int{"2.5": 250, "2.50": 250, "0.6": 60, "0.40": 40, "100": 10000, "0": 0, "0.01": 1}
	for in, want := range cases {
		got, err := PercentToBPS(in)
		if err != nil || got != want {
			t.Fatalf("PercentToBPS(%q)=%d,%v want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"100.01", "2.555", "-1", ""} {
		if _, err := PercentToBPS(bad); err == nil {
			t.Fatalf("PercentToBPS(%q) want error", bad)
		}
	}
}

func TestMulBPSHalfUp(t *testing.T) {
	cases := []struct {
		amount int64
		bps    int
		want   int64
	}{
		{10000, 250, 250},
		{1, 1, 0},
		{1, 5000, 1},
		{1999, 250, 50}, // 49.975 fen -> 50
		{100, 0, 0},
		{333, 10000, 333},
		{10, 333, 0},
		{15, 5000, 8}, // 7.5 -> 8
		{0, 250, 0},
	}
	for _, tc := range cases {
		got, err := MulBPS(tc.amount, tc.bps)
		if err != nil || got != tc.want {
			t.Fatalf("MulBPS(%d,%d)=%d,%v want %d", tc.amount, tc.bps, got, err, tc.want)
		}
	}
	if _, err := MulBPS(10, 10001); err == nil {
		t.Fatal("expected rate error")
	}
	if _, err := MulBPS(-1, 10); err == nil {
		t.Fatal("expected rate error")
	}
}

func TestResolveSeatPriceAndAdditionalAccounts(t *testing.T) {
	over := int64(100)
	got, err := ResolveSeatPrice(19900, &over)
	if err != nil || got != 100 {
		t.Fatalf("override: %d %v", got, err)
	}
	got, err = ResolveSeatPrice(19900, nil)
	if err != nil || got != 19900 {
		t.Fatalf("catalog: %d %v", got, err)
	}
	// First account bought at 199.00. Admin then raises the seat price.
	// The second Alipay account is a new buyout at the new price.
	first, err := ResolveSeatPrice(19900, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondCatalog := int64(29900)
	second, err := ResolveSeatPrice(secondCatalog, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first != 19900 || second != 29900 {
		t.Fatalf("snapshots changed: %d %d", first, second)
	}
	total, err := SumCents(first, second)
	if err != nil || total != 49800 {
		t.Fatalf("total %d %v", total, err)
	}
	if _, err := SumCents(1, -1); err == nil {
		t.Fatal("expected negative rejection")
	}
}

func TestPhase1BuyoutModes(t *testing.T) {
	if err := ValidatePhase1Buyout(BillingOneTime); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePhase1Buyout(BillingDuration); err == nil {
		t.Fatal("duration should be reserved")
	}
	if err := ValidatePhase1Buyout(BillingQuantity); err == nil {
		t.Fatal("quantity should be reserved")
	}
	if err := ValidatePhase1Buyout("POINTS"); err == nil {
		t.Fatal("unknown mode")
	}
}
