package alipay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/yin/alipay-nowpay-gateway/internal/id"
	"github.com/yin/alipay-nowpay-gateway/internal/money"
)

// CallError is a business failure from the mock gateway or simulator.
type CallError struct {
	SubCode string
	SubMsg  string
}

func (e *CallError) Error() string { return e.SubMsg }

func abort(code, msg string) error { return &CallError{SubCode: code, SubMsg: msg} }

type MockOptions struct {
	PublicBaseURL string
	Now           func() time.Time
}

type Mock struct {
	store StateStore
	base  string
	now   func() time.Time
}

func NewMock(store StateStore, opt MockOptions) *Mock {
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	return &Mock{store: store, base: strings.TrimRight(opt.PublicBaseURL, "/"), now: now}
}

func (m *Mock) ChargeQuery(ctx context.Context, biz ChargeQueryBiz) (Result[ChargeQueryBody], error) {
	if biz.OutProductID == "" || biz.ExternalOwnerID == "" {
		return fail[ChargeQueryBody]("INVALID_PARAMETER", "out_product_id 与 external_owner_id 必填"), nil
	}
	rec, err := m.store.GetCharge(ctx, biz.OutProductID, biz.ExternalOwnerID)
	if err != nil {
		return Result[ChargeQueryBody]{}, err
	}
	if rec == nil {
		return ok(ChargeQueryBody{CapabilityStatus: StatusNotConfigured, ChargingOptions: []ChargingOption{}}), nil
	}
	return ok(chargeBody(rec)), nil
}

func (m *Mock) ChargeInitialize(ctx context.Context, biz ChargeInitializeBiz) (Result[ChargeInitializeBody], error) {
	if biz.OutProductID == "" || biz.ExternalOwnerID == "" || biz.ProductName == "" {
		return fail[ChargeInitializeBody]("INVALID_PARAMETER", "商品、创作者与名称必填"), nil
	}
	if len(biz.PricingModeList) == 0 {
		return fail[ChargeInitializeBody]("INVALID_PARAMETER", "pricing_mode_list 至少一项"), nil
	}
	for _, mode := range biz.PricingModeList {
		switch mode.BillingMode {
		case BillingOneTime, BillingDuration:
		case BillingQuantity:
			if mode.QuotaUnit != "COUNT" && mode.QuotaUnit != "POINT" {
				return fail[ChargeInitializeBody]("INVALID_PARAMETER", "QUANTITY 需要 quota_unit COUNT 或 POINT"), nil
			}
		default:
			return fail[ChargeInitializeBody]("INVALID_PARAMETER", "未知 billing_mode"), nil
		}
	}
	if err := validateCallback(biz.CallbackURL); err != nil {
		return fail[ChargeInitializeBody]("INVALID_PARAMETER", err.Error()), nil
	}
	rec, err := m.store.MutateCharge(ctx, biz.OutProductID, biz.ExternalOwnerID, func(cur *ChargeRecord) (*ChargeRecord, error) {
		if cur != nil && cur.CapabilityStatus != "" && cur.CapabilityStatus != StatusNotConfigured && cur.CapabilityStatus != StatusConfiguring {
			return nil, abort("NOW_PAY_ALREADY_CONFIGURED", "商品已完成初始化配置，无法重复配置")
		}
		if cur != nil && cur.CapabilityStatus == StatusConfiguring && cur.Ticket != "" {
			cur.ProductName = biz.ProductName
			cur.CallbackURL = biz.CallbackURL
			cur.PricingModes = append([]PricingMode(nil), biz.PricingModeList...)
			return cur, nil
		}
		ticket := id.Token(16)
		next := &ChargeRecord{
			OutProductID:        biz.OutProductID,
			ExternalOwnerID:     biz.ExternalOwnerID,
			ProductName:         biz.ProductName,
			ProductIconURL:      biz.ProductIconURL,
			ProductURL:          biz.ProductURL,
			CallbackURL:         biz.CallbackURL,
			PricingModes:        append([]PricingMode(nil), biz.PricingModeList...),
			CapabilityStatus:    StatusConfiguring,
			Options:             []ChargingOption{},
			ConfigurationURL:    m.appLink("pages/now-pay/config/index", ticket),
			ConfigurationQRCode: m.pageURL("/mock/alipay/config", ticket),
			Ticket:              ticket,
		}
		return next, nil
	})
	if err != nil {
		return asResult[ChargeInitializeBody](err)
	}
	return ok(ChargeInitializeBody{
		ConfigurationURL:    rec.ConfigurationURL,
		ConfigurationQRCode: rec.ConfigurationQRCode,
	}), nil
}

func (m *Mock) ChargeModify(ctx context.Context, biz ChargeModifyBiz) (Result[ChargeModifyBody], error) {
	if biz.Action != ActionEnable && biz.Action != ActionDisable {
		return fail[ChargeModifyBody]("INVALID_PARAMETER", "action 仅支持 ENABLE 或 DISABLE"), nil
	}
	before, err := m.store.GetCharge(ctx, biz.OutProductID, biz.ExternalOwnerID)
	if err != nil {
		return Result[ChargeModifyBody]{}, err
	}
	beforeVersion := 0
	if before != nil {
		beforeVersion = before.StatusVersion
	}
	rec, err := m.store.MutateCharge(ctx, biz.OutProductID, biz.ExternalOwnerID, func(cur *ChargeRecord) (*ChargeRecord, error) {
		if cur == nil || cur.CapabilityStatus == StatusNotConfigured || cur.CapabilityStatus == StatusConfiguring {
			return nil, abort("NOW_PAY_BINDING_NOT_FOUND", "未找到已配置的商品")
		}
		if cur.CapabilityStatus != StatusEnabled && cur.CapabilityStatus != StatusDisabled {
			return nil, abort("NOW_PAY_CAPABILITY_CHANGE_FAIL", "当前状态不允许启停")
		}
		target := StatusEnabled
		if biz.Action == ActionDisable {
			target = StatusDisabled
		}
		if cur.CapabilityStatus == target {
			return cur, nil
		}
		cur.CapabilityStatus = target
		cur.StatusVersion++
		return cur, nil
	})
	if err != nil {
		return asResult[ChargeModifyBody](err)
	}
	return ok(ChargeModifyBody{
		CapabilityStatus: rec.CapabilityStatus,
		StatusVersion:    rec.StatusVersion,
		Changed:          rec.StatusVersion != beforeVersion,
	}), nil
}

func (m *Mock) PurchaseConsult(ctx context.Context, biz PurchaseConsultBiz) (Result[PurchaseConsultBody], error) {
	if biz.OutProductID == "" || biz.ExternalOwnerID == "" || biz.ExternalBuyerID == "" {
		return fail[PurchaseConsultBody]("INVALID_PARAMETER", "商品、创作者与买家标识必填"), nil
	}
	rec, err := m.store.GetCharge(ctx, biz.OutProductID, biz.ExternalOwnerID)
	if err != nil {
		return Result[PurchaseConsultBody]{}, err
	}
	if rec == nil {
		return ok(PurchaseConsultBody{Decision: DecisionDeny, ReasonCode: "NOT_CONFIGURED", CapabilityStatus: StatusNotConfigured, ChargingOptions: []ChargingOption{}}), nil
	}
	if rec.BillingMode == BillingQuantity {
		return fail[PurchaseConsultBody]("BILLING_MODE_NOT_SUPPORTED", "额度包请改用 quota.query"), nil
	}
	ent, err := m.store.GetEntitlement(ctx, biz.OutProductID, biz.ExternalOwnerID, biz.ExternalBuyerID)
	if err != nil {
		return Result[PurchaseConsultBody]{}, err
	}
	body := PurchaseConsultBody{
		ProductName:      rec.ProductName,
		ProductIconURL:   rec.ProductIconURL,
		CapabilityStatus: rec.CapabilityStatus,
		ChargingOptions:  nonNilOptions(rec.Options),
	}
	if entitlementActive(ent, m.now()) {
		body.Decision = DecisionAllow
		if ent.ValidFrom != nil {
			body.ValidFrom = formatShanghai(*ent.ValidFrom)
		}
		if ent.ValidUntil != nil {
			body.ValidUntil = formatShanghai(*ent.ValidUntil)
		}
		return ok(body), nil
	}
	if rec.CapabilityStatus != StatusEnabled {
		body.Decision = DecisionDeny
		body.ReasonCode = denyReason(rec.CapabilityStatus)
		body.ChargingOptions = nonNilOptions(nil)
		return ok(body), nil
	}
	link, err := m.ensureConsultLink(ctx, biz, rec)
	if err != nil {
		return asResult[PurchaseConsultBody](err)
	}
	body.Decision = DecisionPurchase
	body.PurchaseURL = link.PurchaseURL
	body.PurchaseQRCode = link.PurchaseQRCode
	return ok(body), nil
}

func (m *Mock) PurchaseCreate(ctx context.Context, biz PurchaseCreateBiz) (Result[PurchaseCreateBody], error) {
	if biz.OutRequestNo == "" || biz.OutProductID == "" || biz.ExternalOwnerID == "" || biz.ExternalBuyerID == "" {
		return fail[PurchaseCreateBody]("INVALID_PARAMETER", "请求号、商品、创作者与买家标识必填"), nil
	}
	if err := validateCallback(biz.CallbackURL); err != nil {
		return fail[PurchaseCreateBody]("INVALID_PARAMETER", err.Error()), nil
	}
	if existing, err := m.store.GetLink(ctx, biz.OutRequestNo); err != nil {
		return Result[PurchaseCreateBody]{}, err
	} else if existing != nil {
		if existing.Fingerprint != fingerprint(biz) {
			return fail[PurchaseCreateBody]("NOW_PAY_ORDER_CONFLICT", "同一幂等请求号对应的业务参数不一致"), nil
		}
		if existing.Paid || existing.ExpireAt.After(m.now()) {
			return ok(PurchaseCreateBody{
				PurchaseURL:    existing.PurchaseURL,
				PurchaseQRCode: existing.PurchaseQRCode,
				ExpireTime:     formatShanghai(existing.ExpireAt),
				ChargingOption: existing.Option,
			}), nil
		}
		return fail[PurchaseCreateBody]("NOW_PAY_PURCHASE_LINK_INVALID", "购买链接已过期，请换新的请求号"), nil
	}
	rec, err := m.store.GetCharge(ctx, biz.OutProductID, biz.ExternalOwnerID)
	if err != nil {
		return Result[PurchaseCreateBody]{}, err
	}
	if rec == nil || rec.CapabilityStatus == StatusNotConfigured || rec.CapabilityStatus == StatusConfiguring {
		return fail[PurchaseCreateBody]("NOW_PAY_BINDING_INVALID", "请先完成商品配置上架"), nil
	}
	if rec.CapabilityStatus != StatusEnabled {
		return fail[PurchaseCreateBody]("NOW_PAY_PRODUCT_NOT_PURCHASABLE", "商品当前不可新购"), nil
	}
	ent, err := m.store.GetEntitlement(ctx, biz.OutProductID, biz.ExternalOwnerID, biz.ExternalBuyerID)
	if err != nil {
		return Result[PurchaseCreateBody]{}, err
	}
	if entitlementActive(ent, m.now()) && (rec.BillingMode == BillingOneTime || rec.BillingMode == BillingDuration || rec.BillingMode == "") {
		return fail[PurchaseCreateBody]("NOW_PAY_PRODUCT_NOT_PURCHASABLE", "用户已有不可重购权益"), nil
	}
	var picked *ChargingOption
	if biz.SKUID != "" {
		for i := range rec.Options {
			if rec.Options[i].SKUID == biz.SKUID {
				opt := rec.Options[i]
				picked = &opt
				break
			}
		}
		if picked == nil {
			return fail[PurchaseCreateBody]("NOW_PAY_CHARGING_OPTION_UNAVAILABLE", "指定档位已下架或不存在"), nil
		}
	}
	fp := fingerprint(biz)
	link, err := m.store.MutateLink(ctx, biz.OutRequestNo, func(cur *LinkRecord) (*LinkRecord, error) {
		if cur != nil {
			if cur.Fingerprint != fp {
				return nil, abort("NOW_PAY_ORDER_CONFLICT", "同一幂等请求号对应的业务参数不一致")
			}
			if !cur.Paid && !cur.ExpireAt.After(m.now()) {
				return nil, abort("NOW_PAY_PURCHASE_LINK_INVALID", "购买链接已过期，请换新的请求号")
			}
			return cur, nil
		}
		ticket := id.Token(16)
		expire := m.now().Add(30 * time.Minute)
		return &LinkRecord{
			OutRequestNo:    biz.OutRequestNo,
			Fingerprint:     fp,
			OutProductID:    biz.OutProductID,
			ExternalOwnerID: biz.ExternalOwnerID,
			ExternalBuyerID: biz.ExternalBuyerID,
			SKUID:           biz.SKUID,
			CallbackURL:     biz.CallbackURL,
			PurchaseURL:     m.appLink("pages/order/index", ticket),
			PurchaseQRCode:  m.pageURL("/mock/alipay/purchase", ticket),
			ExpireAt:        expire,
			Option:          picked,
			Ticket:          ticket,
		}, nil
	})
	if err != nil {
		return asResult[PurchaseCreateBody](err)
	}
	return ok(PurchaseCreateBody{
		PurchaseURL:    link.PurchaseURL,
		PurchaseQRCode: link.PurchaseQRCode,
		ExpireTime:     formatShanghai(link.ExpireAt),
		ChargingOption: link.Option,
	}), nil
}

func (m *Mock) QuotaQuery(ctx context.Context, biz QuotaQueryBiz) (Result[QuotaQueryBody], error) {
	return rejectQuota[QuotaQueryBody](m, ctx, biz.OutProductID, biz.ExternalOwnerID)
}

func (m *Mock) QuotaVerify(ctx context.Context, biz QuotaVerifyBiz) (Result[QuotaConsumeBody], error) {
	return rejectQuota[QuotaConsumeBody](m, ctx, biz.OutProductID, biz.ExternalOwnerID)
}

func (m *Mock) QuotaRefresh(ctx context.Context, biz QuotaRefreshBiz) (Result[QuotaConsumeBody], error) {
	return rejectQuota[QuotaConsumeBody](m, ctx, biz.OutProductID, biz.ExternalOwnerID)
}

func rejectQuota[T any](m *Mock, ctx context.Context, product, owner string) (Result[T], error) {
	rec, err := m.store.GetCharge(ctx, product, owner)
	if err != nil {
		return Result[T]{}, err
	}
	if rec == nil || rec.BillingMode != BillingQuantity {
		return fail[T]("BILLING_MODE_NOT_SUPPORTED", "商品不是额度包模式"), nil
	}
	return fail[T]("SYSTEM_ERROR", "额度包核销模拟留到后续阶段，阶段一只完成永久买断"), nil
}

func (m *Mock) CompleteConfiguration(ctx context.Context, in CompleteConfigInput) (Result[ChargeQueryBody], error) {
	cents, err := money.ParseYuan(in.PriceYuan)
	if err != nil || cents < 1 {
		return fail[ChargeQueryBody]("INVALID_PARAMETER", "托管页需要填写至少 0.01 元的档位价格"), nil
	}
	price := money.FormatYuan(cents)
	rec, err := m.store.MutateCharge(ctx, in.OutProductID, in.ExternalOwnerID, func(cur *ChargeRecord) (*ChargeRecord, error) {
		if cur == nil || cur.CapabilityStatus == StatusNotConfigured {
			return nil, abort("NOW_PAY_BINDING_NOT_FOUND", "请先初始化收费配置")
		}
		if cur.CapabilityStatus == StatusEnabled || cur.CapabilityStatus == StatusDisabled {
			return cur, nil
		}
		if cur.CapabilityStatus != StatusConfiguring {
			return nil, abort("NOW_PAY_CAPABILITY_CHANGE_FAIL", "当前状态不能完成配置")
		}
		mode := BillingOneTime
		if len(cur.PricingModes) > 0 && cur.PricingModes[0].BillingMode != "" {
			mode = cur.PricingModes[0].BillingMode
		}
		if mode != BillingOneTime {
			return nil, abort("BILLING_MODE_NOT_SUPPORTED", "阶段一模拟托管页只完成永久买断 ONE_TIME")
		}
		name := in.DisplayName
		if name == "" {
			name = "永久买断"
		}
		logon := maskLogon(in.LogonID)
		userID := "2088" + digits(12)
		if logon == "" {
			logon = maskLogon("user" + digits(8))
		}
		cur.CapabilityStatus = StatusEnabled
		cur.BillingMode = mode
		cur.StatusVersion++
		cur.AlipayUserID = userID
		cur.LogonMasked = logon
		cur.ManagementURL = m.appLink("pages/now-pay/product-detail/index", cur.Ticket)
		cur.Options = []ChargingOption{{
			SKUID:           "S" + digits(16),
			PlanID:          "PI" + digits(16),
			DisplayName:     name,
			BillingMode:     BillingOneTime,
			Price:           price,
			Currency:        "CNY",
			PriceVersion:    1,
			EntitlementRule: EntitlementPerpetual,
		}}
		return cur, nil
	})
	if err != nil {
		return asResult[ChargeQueryBody](err)
	}
	return ok(chargeBody(rec)), nil
}

func (m *Mock) MarkPurchasePaid(ctx context.Context, outRequestNo string) (MarkPaidResult, error) {
	existing, err := m.store.GetLink(ctx, outRequestNo)
	if err != nil {
		return MarkPaidResult{}, err
	}
	if existing == nil {
		return MarkPaidResult{}, &CallError{SubCode: "NOW_PAY_PURCHASE_LINK_INVALID", SubMsg: "购买链接不存在"}
	}
	if !existing.Paid {
		ent, err := m.store.GetEntitlement(ctx, existing.OutProductID, existing.ExternalOwnerID, existing.ExternalBuyerID)
		if err != nil {
			return MarkPaidResult{}, err
		}
		if entitlementActive(ent, m.now()) {
			return MarkPaidResult{}, &CallError{SubCode: "NOW_PAY_PRODUCT_NOT_PURCHASABLE", SubMsg: "用户已有不可重购权益"}
		}
	}
	var result MarkPaidResult
	link, err := m.store.MutateLink(ctx, outRequestNo, func(cur *LinkRecord) (*LinkRecord, error) {
		if cur == nil {
			return nil, abort("NOW_PAY_PURCHASE_LINK_INVALID", "购买链接不存在")
		}
		if cur.Paid {
			result = paidResult(cur, true)
			return cur, nil
		}
		if !cur.ExpireAt.After(m.now()) {
			return nil, abort("NOW_PAY_PURCHASE_LINK_INVALID", "购买链接已过期")
		}
		if cur.Option == nil {
			return nil, abort("INVALID_PARAMETER", "未锁定档位，无法确定付款金额")
		}
		cents, err := money.ParseYuan(cur.Option.Price)
		if err != nil {
			return nil, abort("INVALID_PARAMETER", "档位价格无法解析")
		}
		now := m.now()
		cur.Paid = true
		cur.PaidAt = &now
		cur.TradeNo = "MOCK" + digits(24)
		result = paidResult(cur, false)
		result.GrossCents = cents
		return cur, nil
	})
	if err != nil {
		var b *CallError
		if errors.As(err, &b) {
			return MarkPaidResult{}, b
		}
		return MarkPaidResult{}, err
	}
	if link.Option != nil {
		if cents, err := money.ParseYuan(link.Option.Price); err == nil {
			result.GrossCents = cents
		}
	}
	if err := m.store.SaveEntitlement(ctx, EntitlementRecord{
		OutProductID:    link.OutProductID,
		ExternalOwnerID: link.ExternalOwnerID,
		ExternalBuyerID: link.ExternalBuyerID,
		BillingMode:     result.BillingMode,
		SKUID:           link.SKUID,
	}); err != nil {
		return MarkPaidResult{}, err
	}
	return result, nil
}

func (m *Mock) Account(ctx context.Context, product, owner string) (string, string, error) {
	rec, err := m.store.GetCharge(ctx, product, owner)
	if err != nil {
		return "", "", err
	}
	if rec == nil {
		return "", "", abort("NOW_PAY_BINDING_NOT_FOUND", "尚未初始化收费配置")
	}
	return rec.AlipayUserID, rec.LogonMasked, nil
}

func (m *Mock) LookupTicket(ctx context.Context, ticket string) (TicketView, error) {
	if ticket == "" {
		return TicketView{}, abort("INVALID_PARAMETER", "缺少 ticket")
	}
	if ch, err := m.store.GetChargeByTicket(ctx, ticket); err != nil {
		return TicketView{}, err
	} else if ch != nil {
		price, name, currency := "", "", "CNY"
		if len(ch.Options) > 0 {
			price = ch.Options[0].Price
			name = ch.Options[0].DisplayName
			currency = ch.Options[0].Currency
		}
		return TicketView{
			Kind:             "config",
			Ticket:           ticket,
			OutProductID:     ch.OutProductID,
			ExternalOwnerID:  ch.ExternalOwnerID,
			ProductName:      ch.ProductName,
			Price:            price,
			Currency:         currency,
			DisplayName:      name,
			CapabilityStatus: ch.CapabilityStatus,
			LogonMasked:      ch.LogonMasked,
			CallbackURL:      ch.CallbackURL,
			Paid:             ch.CapabilityStatus == StatusEnabled || ch.CapabilityStatus == StatusDisabled,
		}, nil
	}
	link, err := m.store.GetLinkByTicket(ctx, ticket)
	if err != nil {
		return TicketView{}, err
	}
	if link == nil {
		return TicketView{}, abort("NOW_PAY_PURCHASE_LINK_INVALID", "找不到这张模拟托管页")
	}
	ch, err := m.store.GetCharge(ctx, link.OutProductID, link.ExternalOwnerID)
	if err != nil {
		return TicketView{}, err
	}
	view := TicketView{
		Kind:            "purchase",
		Ticket:          ticket,
		OutRequestNo:    link.OutRequestNo,
		OutProductID:    link.OutProductID,
		ExternalOwnerID: link.ExternalOwnerID,
		ExternalBuyerID: link.ExternalBuyerID,
		Paid:            link.Paid,
		Expired:         !link.Paid && !link.ExpireAt.After(m.now()),
		ExpireAt:        link.ExpireAt,
		CallbackURL:     link.CallbackURL,
	}
	if ch != nil {
		view.ProductName = ch.ProductName
		view.CapabilityStatus = ch.CapabilityStatus
	}
	if link.Option != nil {
		view.Price = link.Option.Price
		view.Currency = link.Option.Currency
		view.DisplayName = link.Option.DisplayName
	}
	return view, nil
}

func (m *Mock) ensureConsultLink(ctx context.Context, biz PurchaseConsultBiz, rec *ChargeRecord) (*LinkRecord, error) {
	sum := sha256.Sum256([]byte(biz.OutProductID + "|" + biz.ExternalOwnerID + "|" + biz.ExternalBuyerID))
	reqNo := "consult_" + hex.EncodeToString(sum[:12])
	sku := ""
	var opt *ChargingOption
	if len(rec.Options) > 0 {
		o := rec.Options[0]
		opt = &o
		sku = o.SKUID
	}
	fp := fingerprint(PurchaseCreateBiz{
		OutRequestNo:    reqNo,
		OutProductID:    biz.OutProductID,
		ExternalOwnerID: biz.ExternalOwnerID,
		ExternalBuyerID: biz.ExternalBuyerID,
		SKUID:           sku,
		CallbackURL:     biz.CallbackURL,
	})
	return m.store.MutateLink(ctx, reqNo, func(cur *LinkRecord) (*LinkRecord, error) {
		if cur != nil && (cur.Paid || cur.ExpireAt.After(m.now())) {
			return cur, nil
		}
		ticket := id.Token(16)
		if cur != nil && cur.Ticket != "" {
			ticket = cur.Ticket
		}
		return &LinkRecord{
			OutRequestNo:    reqNo,
			Fingerprint:     fp,
			OutProductID:    biz.OutProductID,
			ExternalOwnerID: biz.ExternalOwnerID,
			ExternalBuyerID: biz.ExternalBuyerID,
			SKUID:           sku,
			CallbackURL:     biz.CallbackURL,
			PurchaseURL:     m.appLink("pages/order/index", ticket),
			PurchaseQRCode:  m.pageURL("/mock/alipay/purchase", ticket),
			ExpireAt:        m.now().Add(30 * time.Minute),
			Option:          opt,
			Ticket:          ticket,
		}, nil
	})
}

func paidResult(link *LinkRecord, already bool) MarkPaidResult {
	mode := BillingOneTime
	if link.Option != nil && link.Option.BillingMode != "" {
		mode = link.Option.BillingMode
	}
	return MarkPaidResult{
		TradeNo:         link.TradeNo,
		OutProductID:    link.OutProductID,
		ExternalOwnerID: link.ExternalOwnerID,
		ExternalBuyerID: link.ExternalBuyerID,
		SKUID:           link.SKUID,
		BillingMode:     mode,
		Already:         already,
	}
}

func chargeBody(rec *ChargeRecord) ChargeQueryBody {
	return ChargeQueryBody{
		CapabilityStatus: rec.CapabilityStatus,
		ProductName:      rec.ProductName,
		ProductIconURL:   rec.ProductIconURL,
		StatusVersion:    rec.StatusVersion,
		BillingMode:      rec.BillingMode,
		QuotaUnit:        rec.QuotaUnit,
		ChargingOptions:  nonNilOptions(rec.Options),
		ManagementURL:    rec.ManagementURL,
		ReasonMessage:    rec.ReasonMessage,
	}
}

func nonNilOptions(in []ChargingOption) []ChargingOption {
	if in == nil {
		return []ChargingOption{}
	}
	return in
}

func entitlementActive(ent *EntitlementRecord, now time.Time) bool {
	if ent == nil {
		return false
	}
	if ent.ValidUntil != nil && !now.Before(*ent.ValidUntil) {
		return false
	}
	return true
}

func denyReason(status string) string {
	switch status {
	case StatusDisabled:
		return "CAPABILITY_DISABLED"
	case StatusConfiguring:
		return "CONFIGURING"
	case StatusNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return "CAPABILITY_NOT_ENABLED"
	}
}

func validateCallback(raw string) error {
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") {
		return errors.New("callback_url 需要是 http(s) 地址")
	}
	return nil
}

func fingerprint(biz PurchaseCreateBiz) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		biz.OutRequestNo, biz.OutProductID, biz.ExternalOwnerID, biz.ExternalBuyerID, biz.SKUID, biz.CallbackURL,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (m *Mock) pageURL(path, ticket string) string {
	if m.base == "" {
		return path + "?ticket=" + ticket
	}
	return m.base + path + "?ticket=" + ticket
}

func (m *Mock) appLink(page, ticket string) string {
	return "alipays://platformapi/startapp?appId=MOCK_NOWPAY&page=" + page + "&ticket=" + ticket
}

var shanghai = time.FixedZone("Asia/Shanghai", 8*3600)

func formatShanghai(t time.Time) string {
	return t.In(shanghai).Format("2006-01-02 15:04:05")
}

// ParseShanghai reads an official gateway timestamp (Asia/Shanghai, no zone suffix).
func ParseShanghai(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	return time.ParseInLocation("2006-01-02 15:04:05", s, shanghai)
}

func maskLogon(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	if len(r) >= 7 {
		return string(r[:3]) + "****" + string(r[len(r)-4:])
	}
	return string(r[:1]) + "****"
}

func digits(n int) string {
	const d = "0123456789"
	buf := make([]byte, n)
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	for i := 0; i < n; i++ {
		buf[i] = d[int(raw[i])%10]
	}
	return string(buf)
}

func ok[T any](body T) Result[T] {
	return Result[T]{Code: CodeSuccess, Msg: "Success", Sign: "MOCK_SIGN", OK: true, Body: body}
}

func fail[T any](sub, msg string) Result[T] {
	return Result[T]{Code: CodeBusiness, Msg: "Business Failed", SubCode: sub, SubMsg: msg, Sign: "MOCK_SIGN", OK: false}
}

func asResult[T any](err error) (Result[T], error) {
	var b *CallError
	if errors.As(err, &b) {
		return fail[T](b.SubCode, b.SubMsg), nil
	}
	return Result[T]{}, err
}
