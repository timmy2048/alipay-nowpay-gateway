package commission

import "testing"

func TestPlanBalancesAndRounding(t *testing.T) {
	p, err := PlanReceipt(Input{GrossCents: 10000, CommissionBPS: 250, AlipayJSAPIBPS: 60, AlipayNowPayBPS: 40})
	if err != nil {
		t.Fatal(err)
	}
	if p.CommissionCents != 250 || p.AlipayJSAPIFeeCents != 60 || p.AlipayNowPayFeeCents != 40 || p.AlipayFeeCents != 100 {
		t.Fatalf("fees %+v", p)
	}
	if p.CreatorNetCents != 9650 || p.EstimatesExceedGross {
		t.Fatalf("net %+v", p)
	}
	var dr, cr int64
	for _, ln := range p.Lines {
		if ln.Status == StatusMemo {
			continue
		}
		if ln.Direction == DirectionDebit {
			dr += ln.Amount
		} else {
			cr += ln.Amount
		}
	}
	if dr != cr || dr != 250 {
		t.Fatalf("cash lines unbalanced dr=%d cr=%d", dr, cr)
	}
}

func TestPlanZeroCommissionStillMemosGross(t *testing.T) {
	p, err := PlanReceipt(Input{GrossCents: 500, CommissionBPS: 0, AlipayJSAPIBPS: 60, AlipayNowPayBPS: 40})
	if err != nil {
		t.Fatal(err)
	}
	if p.CommissionCents != 0 || p.CreatorNetCents != 495 {
		t.Fatalf("%+v", p)
	}
	for _, ln := range p.Lines {
		if ln.Account == AccountReceivable {
			t.Fatal("zero commission should not book a receivable")
		}
	}
}

func TestPlanFloorsNetWhenEstimatesExceedGross(t *testing.T) {
	p, err := PlanReceipt(Input{GrossCents: 1, CommissionBPS: 10000, AlipayJSAPIBPS: 10000, AlipayNowPayBPS: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if p.CommissionCents != 1 || p.AlipayFeeCents != 2 || p.CreatorNetCents != 0 || !p.EstimatesExceedGross {
		t.Fatalf("%+v", p)
	}
}

func TestEngineName(t *testing.T) {
	e, err := New("")
	if err != nil || e.Name() != EngineMock {
		t.Fatalf("%v %v", e, err)
	}
	if _, err := New("alipay_split"); err == nil {
		t.Fatal("expected unknown engine")
	}
}
