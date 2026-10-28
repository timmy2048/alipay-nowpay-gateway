package store

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrNothingToSettle = errors.New("nothing to settle")
	ErrInactive        = errors.New("inactive")
)

type Actor struct {
	Type          string
	ID            string
	Email         string
	Name          string
	CreatorID     string
	CreatorStatus string
	ExpiresAt     time.Time
}

type Admin struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
}

type Creator struct {
	ID           string    `json:"id"`
	PublicCode   string    `json:"public_code"`
	Name         string    `json:"name"`
	ContactEmail string    `json:"contact_email"`
	Phone        string    `json:"phone"`
	Status       string    `json:"status"`
	Notes        string    `json:"notes"`
	PortalUserID string    `json:"portal_user_id,omitempty"`
	PortalEmail  string    `json:"portal_email,omitempty"`
	PortalName   string    `json:"portal_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type Project struct {
	ID                      string    `json:"id"`
	PublicCode              string    `json:"public_code"`
	CreatorID               string    `json:"creator_id"`
	CreatorName             string    `json:"creator_name"`
	CreatorCode             string    `json:"creator_public_code"`
	Name                    string    `json:"name"`
	Status                  string    `json:"status"`
	CallbackURL             string    `json:"callback_url"`
	NotifyURL               string    `json:"notify_url"`
	BillingMode             string    `json:"billing_mode"`
	SeatPriceCents          *int64    `json:"seat_price_cents"`
	CustomerPriceCents      int64     `json:"customer_price_cents"`
	CommissionBPS           *int      `json:"commission_bps"`
	EffectiveSeatPriceCents int64     `json:"effective_seat_price_cents"`
	EffectiveCommissionBPS  int       `json:"effective_commission_bps"`
	SeatCount               int       `json:"seat_count"`
	BoundCount              int       `json:"bound_count"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type PriceItem struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	BillingMode    string    `json:"billing_mode"`
	QuotaUnit      string    `json:"quota_unit,omitempty"`
	DurationPeriod string    `json:"duration_period,omitempty"`
	AmountCents    int64     `json:"amount_cents"`
	Currency       string    `json:"currency"`
	Active         bool      `json:"active"`
	Purchasable    bool      `json:"purchasable"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CommissionSettings struct {
	RateBPS         int       `json:"rate_bps"`
	AlipayJSAPIBPS  int       `json:"alipay_jsapi_fee_bps"`
	AlipayNowPayBPS int       `json:"alipay_nowpay_fee_bps"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Seat struct {
	ID                  string          `json:"id"`
	PublicCode          string          `json:"public_code"`
	ProjectID           string          `json:"project_id"`
	CreatorID           string          `json:"creator_id"`
	ProjectName         string          `json:"project_name,omitempty"`
	CreatorName         string          `json:"creator_name,omitempty"`
	Ordinal             int             `json:"ordinal"`
	Status              string          `json:"status"`
	BillingMode         string          `json:"billing_mode"`
	AmountCents         int64           `json:"amount_cents"`
	Currency            string          `json:"currency"`
	CapabilityStatus    string          `json:"capability_status"`
	AlipayUserID        string          `json:"alipay_user_id"`
	AlipayLogonMasked   string          `json:"alipay_logon_masked"`
	ConfigurationURL    string          `json:"configuration_url"`
	ConfigurationQRCode string          `json:"configuration_qr_code"`
	ManagementURL       string          `json:"management_url"`
	BillingModeActual   string          `json:"billing_mode_actual"`
	ChargingOptions     json.RawMessage `json:"charging_options"`
	StatusVersion       int             `json:"status_version"`
	BoundAt             *time.Time      `json:"bound_at"`
	CreatedAt           time.Time       `json:"created_at"`
}

type Purchase struct {
	ID                   string     `json:"id"`
	SeatID               string     `json:"seat_id"`
	ProjectID            string     `json:"project_id"`
	CreatorID            string     `json:"creator_id"`
	OutRequestNo         string     `json:"out_request_no"`
	ExternalBuyerID      string     `json:"external_buyer_id"`
	SKUID                string     `json:"sku_id"`
	GrossCents           int64      `json:"gross_cents"`
	Currency             string     `json:"currency"`
	Status               string     `json:"status"`
	Decision             string     `json:"decision"`
	AlipayTradeNo        string     `json:"alipay_trade_no"`
	CommissionCents      int64      `json:"commission_cents"`
	CommissionBPS        int        `json:"commission_bps"`
	AlipayFeeCents       int64      `json:"alipay_fee_cents"`
	CreatorNetCents      int64      `json:"creator_net_cents"`
	EstimatesExceedGross bool       `json:"estimates_exceed_gross"`
	PurchaseURL          string     `json:"purchase_url"`
	PurchaseQRCode       string     `json:"purchase_qr_code"`
	ExpireAt             *time.Time `json:"expire_at"`
	PaidAt               *time.Time `json:"paid_at"`
	CreatedAt            time.Time  `json:"created_at"`
}

type LedgerLine struct {
	Account   string
	Direction string
	Amount    int64
	Status    string
	Memo      string
}

type LedgerEntry struct {
	ID           string    `json:"id"`
	PurchaseID   string    `json:"purchase_id,omitempty"`
	SeatID       string    `json:"seat_id,omitempty"`
	ProjectID    string    `json:"project_id,omitempty"`
	CreatorID    string    `json:"creator_id,omitempty"`
	SettlementID string    `json:"settlement_id,omitempty"`
	Account      string    `json:"account"`
	Direction    string    `json:"direction"`
	AmountCents  int64     `json:"amount_cents"`
	Status       string    `json:"status"`
	Memo         string    `json:"memo"`
	CreatedAt    time.Time `json:"created_at"`
}

type Settlement struct {
	ID         string    `json:"id"`
	TotalCents int64     `json:"total_cents"`
	EntryCount int       `json:"entry_count"`
	Note       string    `json:"note"`
	Engine     string    `json:"engine"`
	SettledAt  time.Time `json:"settled_at"`
}

type Audit struct {
	ID           int64           `json:"id"`
	ActorType    string          `json:"actor_type"`
	ActorID      string          `json:"actor_id,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Detail       json.RawMessage `json:"detail"`
	IP           string          `json:"ip"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Webhook struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id,omitempty"`
	EventType string          `json:"event_type"`
	TargetURL string          `json:"target_url"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	Attempts  int             `json:"attempts"`
	LastError string          `json:"last_error"`
	CreatedAt time.Time       `json:"created_at"`
}

type Need struct {
	SeatID      string `json:"seat_id"`
	Ordinal     int    `json:"ordinal"`
	Status      string `json:"status"`
	AmountCents int64  `json:"amount_cents"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	CreatorName string `json:"creator_name"`
}

type Dashboard struct {
	CreatorCount           int        `json:"creator_count"`
	ProjectCount           int        `json:"project_count"`
	BoundSeats             int        `json:"bound_seats"`
	UnpaidSeats            int        `json:"unpaid_seats"`
	ConfiguringSeats       int        `json:"configuring_seats"`
	PaidPurchaseCount      int        `json:"paid_purchase_count"`
	GrossCents             int64      `json:"gross_cents"`
	CommissionBookedCents  int64      `json:"commission_booked_cents"`
	CommissionSettledCents int64      `json:"commission_settled_cents"`
	Needs                  []Need     `json:"needs"`
	Recent                 []Purchase `json:"recent"`
}

type Capability struct {
	SeatStatus          string
	CapabilityStatus    string
	AlipayUserID        string
	LogonMasked         string
	ConfigurationURL    string
	ConfigurationQRCode string
	ManagementURL       string
	BillingModeActual   string
	Options             json.RawMessage
	StatusVersion       int
	MarkBound           bool
}
