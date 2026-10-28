// Package commission plans platform AutoCommission entries.
//
// Customer money lands in the creator's Alipay account. Alipay deducts its
// own acquiring fee there. This gateway cannot assume an instant split, so
// the default engine only books a receivable and recognizes income. Cash is
// recorded later by Settle, which an operator runs when the platform actually
// collects. A future engine can replace Plan without changing the ledger shape.
package commission

import (
	"context"
	"fmt"

	"github.com/yin/alipay-nowpay-gateway/internal/money"
)

const (
	EngineMock = "mock_book_then_settle"

	AccountReceivable   = "platform_commission_receivable"
	AccountIncome       = "platform_commission_income"
	AccountCash         = "platform_cash"
	AccountCreatorGross = "creator_gross_memo"
	AccountAlipayFee    = "alipay_fee_memo"

	DirectionDebit  = "debit"
	DirectionCredit = "credit"

	StatusBooked  = "booked"
	StatusSettled = "settled"
	StatusMemo    = "memo"
)

type Input struct {
	GrossCents      int64
	CommissionBPS   int
	AlipayJSAPIBPS  int
	AlipayNowPayBPS int
}

type Line struct {
	Account   string
	Direction string
	Amount    int64
	Status    string
	Memo      string
}

type Plan struct {
	CommissionCents      int64
	AlipayJSAPIFeeCents  int64
	AlipayNowPayFeeCents int64
	AlipayFeeCents       int64
	CreatorNetCents      int64
	EstimatesExceedGross bool
	Lines                []Line
}

type Engine interface {
	Name() string
	Plan(ctx context.Context, in Input) (Plan, error)
}

type MockEngine struct{}

func (MockEngine) Name() string { return EngineMock }

func (MockEngine) Plan(_ context.Context, in Input) (Plan, error) {
	return PlanReceipt(in)
}

func New(name string) (Engine, error) {
	switch name {
	case "", EngineMock:
		return MockEngine{}, nil
	default:
		return nil, fmt.Errorf("未知 AUTO_COMMISSION=%s（阶段一只有 %s）", name, EngineMock)
	}
}

// PlanReceipt splits one customer payment.
// Commission and each Alipay fee estimate are rounded independently on the
// gross, half up. Estimates are memos: they are not cash this gateway holds.
// Creator net floors at zero when the configured estimates exceed the gross.
func PlanReceipt(in Input) (Plan, error) {
	if in.GrossCents < 0 {
		return Plan{}, money.ErrAmount
	}
	commission, err := money.MulBPS(in.GrossCents, in.CommissionBPS)
	if err != nil {
		return Plan{}, err
	}
	jsapi, err := money.MulBPS(in.GrossCents, in.AlipayJSAPIBPS)
	if err != nil {
		return Plan{}, err
	}
	nowpay, err := money.MulBPS(in.GrossCents, in.AlipayNowPayBPS)
	if err != nil {
		return Plan{}, err
	}
	fee, err := money.SumCents(jsapi, nowpay)
	if err != nil {
		return Plan{}, err
	}
	deduction, err := money.SumCents(commission, fee)
	if err != nil {
		return Plan{}, err
	}
	net := in.GrossCents - deduction
	exceed := false
	if net < 0 {
		net = 0
		exceed = true
	}
	p := Plan{
		CommissionCents:      commission,
		AlipayJSAPIFeeCents:  jsapi,
		AlipayNowPayFeeCents: nowpay,
		AlipayFeeCents:       fee,
		CreatorNetCents:      net,
		EstimatesExceedGross: exceed,
	}
	if commission > 0 {
		p.Lines = append(p.Lines,
			Line{Account: AccountReceivable, Direction: DirectionDebit, Amount: commission, Status: StatusBooked, Memo: "平台抽成应收，待结算"},
			Line{Account: AccountIncome, Direction: DirectionCredit, Amount: commission, Status: StatusBooked, Memo: "平台抽成收入，先记账"},
		)
	}
	p.Lines = append(p.Lines,
		Line{Account: AccountCreatorGross, Direction: DirectionCredit, Amount: in.GrossCents, Status: StatusMemo, Memo: "顾客付款进入创作者支付宝，平台不经手本金"},
		Line{Account: AccountAlipayFee, Direction: DirectionDebit, Amount: fee, Status: StatusMemo, Memo: "支付宝收单费与即刻收费的估计，仅供展示"},
	)
	return p, nil
}
