package alipay

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestMockOneTimeBindPurchaseAndReplay(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, shanghai)
	m := NewMock(NewMemoryStore(), MockOptions{
		PublicBaseURL: "http://127.0.0.1:43123",
		Now:           func() time.Time { return now },
	})
	ctx := context.Background()
	q, err := m.ChargeQuery(ctx, ChargeQueryBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1"})
	if err != nil || !q.OK || q.Body.CapabilityStatus != StatusNotConfigured {
		t.Fatalf("query empty: %+v %v", q, err)
	}
	initBiz := ChargeInitializeBiz{
		OutProductID: "seat_1", ExternalOwnerID: "cr_1", ProductName: "写作助手 · 收款席位 1",
		CallbackURL:     "https://creator.example/return",
		PricingModeList: []PricingMode{{BillingMode: BillingOneTime, Price: "1.00"}},
	}
	first, err := m.ChargeInitialize(ctx, initBiz)
	if err != nil || !first.OK || first.Body.ConfigurationQRCode == "" || !strings.Contains(first.Body.ConfigurationURL, "alipays://") {
		t.Fatalf("init: %+v %v", first, err)
	}
	second, err := m.ChargeInitialize(ctx, initBiz)
	if err != nil || !second.OK || second.Body.ConfigurationQRCode != first.Body.ConfigurationQRCode {
		t.Fatalf("re-init should reuse the ticket: %+v %v", second, err)
	}
	mid, err := m.ChargeQuery(ctx, ChargeQueryBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1"})
	if err != nil || mid.Body.CapabilityStatus != StatusConfiguring || len(mid.Body.ChargingOptions) != 0 {
		t.Fatalf("price must not be applied at initialize: %+v", mid.Body)
	}
	done, err := m.CompleteConfiguration(ctx, CompleteConfigInput{
		OutProductID: "seat_1", ExternalOwnerID: "cr_1", PriceYuan: "29.90", LogonID: "13800138000",
	})
	if err != nil || !done.OK || done.Body.CapabilityStatus != StatusEnabled {
		t.Fatalf("complete: %+v %v", done, err)
	}
	if done.Body.BillingMode != BillingOneTime || len(done.Body.ChargingOptions) != 1 || done.Body.ChargingOptions[0].Price != "29.90" {
		t.Fatalf("option: %+v", done.Body)
	}
	if done.Body.ChargingOptions[0].EntitlementRule != EntitlementPerpetual {
		t.Fatal(done.Body.ChargingOptions[0].EntitlementRule)
	}
	again, err := m.CompleteConfiguration(ctx, CompleteConfigInput{
		OutProductID: "seat_1", ExternalOwnerID: "cr_1", PriceYuan: "99.00", LogonID: "13900139000",
	})
	if err != nil || !again.OK || again.Body.ChargingOptions[0].Price != "29.90" || again.Body.StatusVersion != done.Body.StatusVersion {
		t.Fatalf("second complete must not reprice: %+v", again.Body)
	}
	dup, err := m.ChargeInitialize(ctx, initBiz)
	if err != nil || dup.OK || dup.SubCode != "NOW_PAY_ALREADY_CONFIGURED" {
		t.Fatalf("dup init: %+v %v", dup, err)
	}

	consult, err := m.PurchaseConsult(ctx, PurchaseConsultBiz{
		OutProductID: "seat_1", ExternalOwnerID: "cr_1", ExternalBuyerID: "buyer_1",
		CallbackURL: "https://creator.example/paid",
	})
	if err != nil || consult.Body.Decision != DecisionPurchase || consult.Body.PurchaseQRCode == "" {
		t.Fatalf("consult: %+v %v", consult, err)
	}
	sku := consult.Body.ChargingOptions[0].SKUID
	created, err := m.PurchaseCreate(ctx, PurchaseCreateBiz{
		OutRequestNo: "purchase_1", OutProductID: "seat_1", ExternalOwnerID: "cr_1",
		ExternalBuyerID: "buyer_1", SKUID: sku, CallbackURL: "https://creator.example/paid",
	})
	if err != nil || !created.OK || created.Body.ChargingOption == nil || created.Body.ExpireTime == "" {
		t.Fatalf("create: %+v %v", created, err)
	}
	conflict, err := m.PurchaseCreate(ctx, PurchaseCreateBiz{
		OutRequestNo: "purchase_1", OutProductID: "seat_1", ExternalOwnerID: "cr_1",
		ExternalBuyerID: "buyer_1", SKUID: "other", CallbackURL: "https://creator.example/paid",
	})
	if err != nil || conflict.OK || conflict.SubCode != "NOW_PAY_ORDER_CONFLICT" {
		t.Fatalf("conflict: %+v %v", conflict, err)
	}
	view, err := m.LookupTicket(ctx, ticketOf(created.Body.PurchaseQRCode))
	if err != nil || view.Kind != "purchase" || view.Price != "29.90" {
		t.Fatalf("ticket: %+v %v", view, err)
	}
	paid, err := m.MarkPurchasePaid(ctx, "purchase_1")
	if err != nil || paid.GrossCents != 2990 || !strings.HasPrefix(paid.TradeNo, "MOCK") {
		t.Fatalf("pay: %+v %v", paid, err)
	}
	againPaid, err := m.MarkPurchasePaid(ctx, "purchase_1")
	if err != nil || !againPaid.Already || againPaid.TradeNo != paid.TradeNo {
		t.Fatalf("replay pay: %+v %v", againPaid, err)
	}
	allow, err := m.PurchaseConsult(ctx, PurchaseConsultBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1", ExternalBuyerID: "buyer_1"})
	if err != nil || allow.Body.Decision != DecisionAllow || allow.Body.PurchaseURL != "" || allow.Body.ValidFrom != "" {
		t.Fatalf("allow: %+v", allow.Body)
	}
	blocked, err := m.PurchaseCreate(ctx, PurchaseCreateBiz{
		OutRequestNo: "purchase_2", OutProductID: "seat_1", ExternalOwnerID: "cr_1", ExternalBuyerID: "buyer_1", SKUID: sku,
	})
	if err != nil || blocked.OK || blocked.SubCode != "NOW_PAY_PRODUCT_NOT_PURCHASABLE" {
		t.Fatalf("second buy: %+v %v", blocked, err)
	}

	off, err := m.ChargeModify(ctx, ChargeModifyBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1", Action: ActionDisable})
	if err != nil || !off.OK || !off.Body.Changed || off.Body.CapabilityStatus != StatusDisabled {
		t.Fatalf("disable: %+v %v", off, err)
	}
	same, err := m.ChargeModify(ctx, ChargeModifyBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1", Action: ActionDisable})
	if err != nil || !same.OK || same.Body.Changed {
		t.Fatalf("idempotent disable: %+v", same)
	}
	still, err := m.PurchaseConsult(ctx, PurchaseConsultBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1", ExternalBuyerID: "buyer_1"})
	if err != nil || still.Body.Decision != DecisionAllow {
		t.Fatalf("existing right: %+v", still.Body)
	}
	newbie, err := m.PurchaseConsult(ctx, PurchaseConsultBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1", ExternalBuyerID: "buyer_2"})
	if err != nil || newbie.Body.Decision != DecisionDeny || newbie.Body.ReasonCode != "CAPABILITY_DISABLED" {
		t.Fatalf("new buyer: %+v", newbie.Body)
	}
	quota, err := m.QuotaQuery(ctx, QuotaQueryBiz{OutProductID: "seat_1", ExternalOwnerID: "cr_1", ExternalBuyerID: "buyer_1"})
	if err != nil || quota.OK || quota.SubCode != "BILLING_MODE_NOT_SUPPORTED" {
		t.Fatalf("quota: %+v %v", quota, err)
	}
}

func TestSignContentAndRSA2RoundTrip(t *testing.T) {
	params := BuildGatewayParams("app", MethodChargeQuery, `{"out_product_id":"p"}`, "2026-10-28 10:00:00", "")
	content := SignContent(params)
	if !strings.Contains(content, "method="+MethodChargeQuery) || strings.Contains(content, "sign=") {
		t.Fatal(content)
	}
	if !strings.HasPrefix(content, "app_id=") {
		t.Fatal(content)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	sig, err := SignRSA2(content, string(privPEM))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRSA2(content, sig, string(pubPEM)); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRSA2(content+"x", sig, string(pubPEM)); err == nil {
		t.Fatal("tampered content verified")
	}
}

func TestLiveDoesNotCallNetwork(t *testing.T) {
	live := NewLive("", "", "", "", "")
	_, err := live.ChargeQuery(context.Background(), ChargeQueryBiz{OutProductID: "p", ExternalOwnerID: "c"})
	if !errorsIsNotReady(err) {
		t.Fatal(err)
	}
	if live.GatewayURL != DefaultGatewayURL {
		t.Fatal(live.GatewayURL)
	}
}

func errorsIsNotReady(err error) bool { return err == ErrNotReady }

func ticketOf(raw string) string {
	i := strings.Index(raw, "ticket=")
	if i < 0 {
		return ""
	}
	return raw[i+len("ticket="):]
}
