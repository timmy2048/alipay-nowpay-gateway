package alipay

import (
	"context"
	"time"
)

// ChargeRecord is one NowPay product binding, keyed by out_product_id + owner.
type ChargeRecord struct {
	OutProductID        string
	ExternalOwnerID     string
	ProductName         string
	ProductIconURL      string
	ProductURL          string
	CallbackURL         string
	PricingModes        []PricingMode
	CapabilityStatus    string
	BillingMode         string
	QuotaUnit           string
	StatusVersion       int
	Options             []ChargingOption
	ConfigurationURL    string
	ConfigurationQRCode string
	ManagementURL       string
	Ticket              string
	AlipayUserID        string
	LogonMasked         string
	ReasonMessage       string
}

type LinkRecord struct {
	OutRequestNo    string
	Fingerprint     string
	OutProductID    string
	ExternalOwnerID string
	ExternalBuyerID string
	SKUID           string
	CallbackURL     string
	PurchaseURL     string
	PurchaseQRCode  string
	ExpireAt        time.Time
	Option          *ChargingOption
	Paid            bool
	TradeNo         string
	PaidAt          *time.Time
	Ticket          string
}

type EntitlementRecord struct {
	OutProductID    string
	ExternalOwnerID string
	ExternalBuyerID string
	BillingMode     string
	SKUID           string
	ValidFrom       *time.Time
	ValidUntil      *time.Time
}

// StateStore persists mock gateway state. Memory is for tests; Postgres is
// what the API process uses so a restart does not forget a bound seat.
type StateStore interface {
	MutateCharge(ctx context.Context, product, owner string, fn func(*ChargeRecord) (*ChargeRecord, error)) (*ChargeRecord, error)
	GetCharge(ctx context.Context, product, owner string) (*ChargeRecord, error)
	GetChargeByTicket(ctx context.Context, ticket string) (*ChargeRecord, error)
	MutateLink(ctx context.Context, outRequestNo string, fn func(*LinkRecord) (*LinkRecord, error)) (*LinkRecord, error)
	GetLink(ctx context.Context, outRequestNo string) (*LinkRecord, error)
	GetLinkByTicket(ctx context.Context, ticket string) (*LinkRecord, error)
	SaveEntitlement(ctx context.Context, e EntitlementRecord) error
	GetEntitlement(ctx context.Context, product, owner, buyer string) (*EntitlementRecord, error)
}

type CompleteConfigInput struct {
	OutProductID    string
	ExternalOwnerID string
	PriceYuan       string
	LogonID         string
	DisplayName     string
}

type MarkPaidResult struct {
	TradeNo         string
	GrossCents      int64
	OutProductID    string
	ExternalOwnerID string
	ExternalBuyerID string
	SKUID           string
	BillingMode     string
	Already         bool
}

type TicketView struct {
	Kind             string
	Ticket           string
	OutRequestNo     string
	OutProductID     string
	ExternalOwnerID  string
	ExternalBuyerID  string
	ProductName      string
	Price            string
	Currency         string
	DisplayName      string
	Paid             bool
	Expired          bool
	ExpireAt         time.Time
	CapabilityStatus string
	LogonMasked      string
	CallbackURL      string
}

// Simulator is the hosted-page stand-in. It is not part of the official
// OpenAPI. The live adapter does not implement it.
type Simulator interface {
	CompleteConfiguration(ctx context.Context, in CompleteConfigInput) (Result[ChargeQueryBody], error)
	MarkPurchasePaid(ctx context.Context, outRequestNo string) (MarkPaidResult, error)
	LookupTicket(ctx context.Context, ticket string) (TicketView, error)
	Account(ctx context.Context, product, owner string) (userID, logonMasked string, err error)
}
