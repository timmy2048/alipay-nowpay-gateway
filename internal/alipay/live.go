package alipay

import (
	"context"
	"errors"
)

// ErrNotReady is returned by every live method until a real SDK call is
// wired. Partnership approval is required before any traffic to
// https://openapi.alipay.com/gateway.do. SignRSA2 and BuildGatewayParams are
// ready for that adapter; this type deliberately performs no network I/O.
var ErrNotReady = errors.New("支付宝正式网关未接通：成为即刻收合作平台后，再用 ALIPAY_APP_ID、应用私钥和支付宝公钥替换 internal/alipay/live.go 中的调用")

type Live struct {
	AppID        string
	PrivateKey   string
	AlipayPublic string
	GatewayURL   string
	AppAuthToken string
}

func NewLive(appID, privateKey, alipayPublic, gateway, appAuthToken string) *Live {
	if gateway == "" {
		gateway = DefaultGatewayURL
	}
	return &Live{
		AppID:        appID,
		PrivateKey:   privateKey,
		AlipayPublic: alipayPublic,
		GatewayURL:   gateway,
		AppAuthToken: appAuthToken,
	}
}

func (l *Live) ChargeQuery(context.Context, ChargeQueryBiz) (Result[ChargeQueryBody], error) {
	return Result[ChargeQueryBody]{}, ErrNotReady
}
func (l *Live) ChargeInitialize(context.Context, ChargeInitializeBiz) (Result[ChargeInitializeBody], error) {
	return Result[ChargeInitializeBody]{}, ErrNotReady
}
func (l *Live) ChargeModify(context.Context, ChargeModifyBiz) (Result[ChargeModifyBody], error) {
	return Result[ChargeModifyBody]{}, ErrNotReady
}
func (l *Live) PurchaseConsult(context.Context, PurchaseConsultBiz) (Result[PurchaseConsultBody], error) {
	return Result[PurchaseConsultBody]{}, ErrNotReady
}
func (l *Live) PurchaseCreate(context.Context, PurchaseCreateBiz) (Result[PurchaseCreateBody], error) {
	return Result[PurchaseCreateBody]{}, ErrNotReady
}
func (l *Live) QuotaQuery(context.Context, QuotaQueryBiz) (Result[QuotaQueryBody], error) {
	return Result[QuotaQueryBody]{}, ErrNotReady
}
func (l *Live) QuotaVerify(context.Context, QuotaVerifyBiz) (Result[QuotaConsumeBody], error) {
	return Result[QuotaConsumeBody]{}, ErrNotReady
}
func (l *Live) QuotaRefresh(context.Context, QuotaRefreshBiz) (Result[QuotaConsumeBody], error) {
	return Result[QuotaConsumeBody]{}, ErrNotReady
}
