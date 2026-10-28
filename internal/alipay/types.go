// Package alipay is the platform-mode NowPay client.
//
// Method names and biz_content fields follow the public 即刻收 platform APIs
// (alipay.aipay.nowpay.*). The mock speaks the same structs a live SDK would
// fill. Phase 1 does not call openapi.alipay.com.
package alipay

import "context"

const (
	MethodChargeQuery      = "alipay.aipay.nowpay.charge.query"
	MethodChargeInitialize = "alipay.aipay.nowpay.charge.initialize"
	MethodChargeModify     = "alipay.aipay.nowpay.charge.modify"
	MethodPurchaseConsult  = "alipay.aipay.nowpay.purchase.consult"
	MethodPurchaseCreate   = "alipay.aipay.nowpay.purchase.create"
	MethodQuotaQuery       = "alipay.aipay.nowpay.quota.query"
	MethodQuotaVerify      = "alipay.aipay.nowpay.quota.verify"
	MethodQuotaRefresh     = "alipay.aipay.nowpay.quota.refresh"
	DefaultGatewayURL      = "https://openapi.alipay.com/gateway.do"

	CodeSuccess  = "10000"
	CodeBusiness = "40004"

	StatusNotConfigured  = "NOT_CONFIGURED"
	StatusConfiguring    = "CONFIGURING"
	StatusPending        = "PENDING"
	StatusActionRequired = "ACTION_REQUIRED"
	StatusEnabled        = "ENABLED"
	StatusDisabled       = "DISABLED"
	StatusTerminated     = "TERMINATED"

	BillingOneTime  = "ONE_TIME"
	BillingDuration = "ONE_TIME_DURATION"
	BillingQuantity = "QUANTITY"

	DecisionAllow    = "ALLOW"
	DecisionPurchase = "PURCHASE_REQUIRED"
	DecisionDeny     = "DENY"

	ActionEnable  = "ENABLE"
	ActionDisable = "DISABLE"

	EntitlementPerpetual = "PERPETUAL"
	EntitlementDuration  = "FIXED_DURATION"
	EntitlementQuota     = "QUOTA"
)

// Result is the gateway envelope. Body holds the business object that official
// responses nest inside alipay_aipay_nowpay_*_response, without repeating code.
type Result[T any] struct {
	Code    string
	Msg     string
	SubCode string
	SubMsg  string
	Sign    string
	OK      bool
	Body    T
}

type PricingMode struct {
	BillingMode string `json:"billing_mode"`
	QuotaUnit   string `json:"quota_unit,omitempty"`
	Price       string `json:"price,omitempty"`
}

type ChargingOption struct {
	SKUID           string `json:"sku_id"`
	PlanID          string `json:"plan_id"`
	DisplayName     string `json:"display_name"`
	BillingMode     string `json:"billing_mode"`
	Price           string `json:"price"`
	Currency        string `json:"currency"`
	PriceVersion    int    `json:"price_version"`
	EntitlementRule string `json:"entitlement_rule"`
	DurationPeriod  string `json:"duration_period,omitempty"`
	QuotaUnit       string `json:"quota_unit,omitempty"`
	QuotaAmount     *int   `json:"quota_amount,omitempty"`
}

type ChargeQueryBiz struct {
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id"`
}

type ChargeQueryBody struct {
	CapabilityStatus string           `json:"capability_status"`
	ProductName      string           `json:"product_name,omitempty"`
	ProductIconURL   string           `json:"product_icon_url,omitempty"`
	StatusVersion    int              `json:"status_version,omitempty"`
	BillingMode      string           `json:"billing_mode,omitempty"`
	QuotaUnit        string           `json:"quota_unit,omitempty"`
	ChargingOptions  []ChargingOption `json:"charging_options"`
	ManagementURL    string           `json:"management_url,omitempty"`
	ReasonMessage    string           `json:"reason_message,omitempty"`
}

type ChargeInitializeBiz struct {
	OutProductID    string        `json:"out_product_id"`
	ExternalOwnerID string        `json:"external_owner_id"`
	ProductName     string        `json:"product_name"`
	ProductIconURL  string        `json:"product_icon_url,omitempty"`
	ProductURL      string        `json:"product_url,omitempty"`
	CallbackURL     string        `json:"callback_url,omitempty"`
	PricingModeList []PricingMode `json:"pricing_mode_list"`
}

type ChargeInitializeBody struct {
	ConfigurationURL    string `json:"configuration_url,omitempty"`
	ConfigurationQRCode string `json:"configuration_qr_code,omitempty"`
	ManagementURL       string `json:"management_url,omitempty"`
}

type ChargeModifyBiz struct {
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id"`
	Action          string `json:"action"`
}

type ChargeModifyBody struct {
	CapabilityStatus string `json:"capability_status"`
	StatusVersion    int    `json:"status_version,omitempty"`
	Changed          bool   `json:"changed"`
	ReasonMessage    string `json:"reason_message,omitempty"`
}

type PurchaseConsultBiz struct {
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id"`
	ExternalBuyerID string `json:"external_buyer_id"`
	CallbackURL     string `json:"callback_url,omitempty"`
}

type PurchaseConsultBody struct {
	Decision         string           `json:"decision"`
	ReasonCode       string           `json:"reason_code,omitempty"`
	ValidFrom        string           `json:"valid_from,omitempty"`
	ValidUntil       string           `json:"valid_until,omitempty"`
	ProductName      string           `json:"product_name,omitempty"`
	ProductIconURL   string           `json:"product_icon_url,omitempty"`
	CapabilityStatus string           `json:"capability_status"`
	PurchaseURL      string           `json:"purchase_url,omitempty"`
	PurchaseQRCode   string           `json:"purchase_qr_code,omitempty"`
	ChargingOptions  []ChargingOption `json:"charging_options"`
}

type PurchaseCreateBiz struct {
	OutRequestNo    string `json:"out_request_no"`
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id"`
	ExternalBuyerID string `json:"external_buyer_id"`
	SKUID           string `json:"sku_id,omitempty"`
	CallbackURL     string `json:"callback_url,omitempty"`
}

type PurchaseCreateBody struct {
	PurchaseURL    string          `json:"purchase_url"`
	PurchaseQRCode string          `json:"purchase_qr_code,omitempty"`
	ExpireTime     string          `json:"expire_time"`
	ChargingOption *ChargingOption `json:"charging_option,omitempty"`
}

type QuotaQueryBiz struct {
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id"`
	ExternalBuyerID string `json:"external_buyer_id"`
}

type QuotaQueryBody struct {
	QuotaUnit string `json:"quota_unit"`
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
	AsOfTime  string `json:"as_of_time"`
}

type QuotaVerifyBiz struct {
	OutRequestNo    string `json:"out_request_no"`
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id,omitempty"`
	ExternalBuyerID string `json:"external_buyer_id"`
	Amount          int64  `json:"amount"`
	ConsumeReason   string `json:"consume_reason"`
}

type QuotaRefreshBiz struct {
	OutRequestNo    string `json:"out_request_no"`
	OutProductID    string `json:"out_product_id"`
	ExternalOwnerID string `json:"external_owner_id,omitempty"`
	ExternalBuyerID string `json:"external_buyer_id"`
}

type QuotaConsumeBody struct {
	ConsumeStatus  string `json:"consume_status"`
	ConsumeOrderID string `json:"consume_order_id,omitempty"`
	ConsumeReason  string `json:"consume_reason,omitempty"`
	QuotaUnit      string `json:"quota_unit"`
	Consumed       int64  `json:"consumed"`
	Remaining      int64  `json:"remaining,omitempty"`
	ReasonCode     string `json:"reason_code,omitempty"`
	UpdateTime     string `json:"update_time"`
}

// Client is what a real Alipay SDK adapter must satisfy. Live calls stay
// unwired until the platform partnership is approved.
type Client interface {
	ChargeQuery(ctx context.Context, biz ChargeQueryBiz) (Result[ChargeQueryBody], error)
	ChargeInitialize(ctx context.Context, biz ChargeInitializeBiz) (Result[ChargeInitializeBody], error)
	ChargeModify(ctx context.Context, biz ChargeModifyBiz) (Result[ChargeModifyBody], error)
	PurchaseConsult(ctx context.Context, biz PurchaseConsultBiz) (Result[PurchaseConsultBody], error)
	PurchaseCreate(ctx context.Context, biz PurchaseCreateBiz) (Result[PurchaseCreateBody], error)
	QuotaQuery(ctx context.Context, biz QuotaQueryBiz) (Result[QuotaQueryBody], error)
	QuotaVerify(ctx context.Context, biz QuotaVerifyBiz) (Result[QuotaConsumeBody], error)
	QuotaRefresh(ctx context.Context, biz QuotaRefreshBiz) (Result[QuotaConsumeBody], error)
}
