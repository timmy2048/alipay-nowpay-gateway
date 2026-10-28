package alipay

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore { return &PGStore{pool: pool} }

func (s *PGStore) MutateCharge(ctx context.Context, product, owner string, fn func(*ChargeRecord) (*ChargeRecord, error)) (*ChargeRecord, error) {
	var rec *ChargeRecord
	err := s.retry(func() error {
		var err error
		rec, err = s.mutateChargeOnce(ctx, product, owner, fn)
		return err
	})
	return rec, err
}

func (s *PGStore) mutateChargeOnce(ctx context.Context, product, owner string, fn func(*ChargeRecord) (*ChargeRecord, error)) (*ChargeRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, err := chargeForUpdate(ctx, tx, product, owner)
	if err != nil {
		return nil, err
	}
	next, err := fn(cur)
	if err != nil {
		return nil, err
	}
	if next == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return cur, nil
	}
	if err := upsertCharge(ctx, tx, next); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *PGStore) GetCharge(ctx context.Context, product, owner string) (*ChargeRecord, error) {
	row := s.pool.QueryRow(ctx, chargeSelect+` WHERE out_product_id=$1 AND external_owner_id=$2`, product, owner)
	rec, err := scanCharge(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func (s *PGStore) GetChargeByTicket(ctx context.Context, ticket string) (*ChargeRecord, error) {
	if ticket == "" {
		return nil, nil
	}
	row := s.pool.QueryRow(ctx, chargeSelect+` WHERE ticket=$1`, ticket)
	rec, err := scanCharge(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func (s *PGStore) MutateLink(ctx context.Context, outRequestNo string, fn func(*LinkRecord) (*LinkRecord, error)) (*LinkRecord, error) {
	var rec *LinkRecord
	err := s.retry(func() error {
		var err error
		rec, err = s.mutateLinkOnce(ctx, outRequestNo, fn)
		return err
	})
	return rec, err
}

func (s *PGStore) mutateLinkOnce(ctx context.Context, outRequestNo string, fn func(*LinkRecord) (*LinkRecord, error)) (*LinkRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	cur, err := linkForUpdate(ctx, tx, outRequestNo)
	if err != nil {
		return nil, err
	}
	next, err := fn(cur)
	if err != nil {
		return nil, err
	}
	if next == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return cur, nil
	}
	if err := upsertLink(ctx, tx, next); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *PGStore) GetLink(ctx context.Context, outRequestNo string) (*LinkRecord, error) {
	row := s.pool.QueryRow(ctx, linkSelect+` WHERE out_request_no=$1`, outRequestNo)
	rec, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func (s *PGStore) GetLinkByTicket(ctx context.Context, ticket string) (*LinkRecord, error) {
	row := s.pool.QueryRow(ctx, linkSelect+` WHERE ticket=$1`, ticket)
	rec, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func (s *PGStore) SaveEntitlement(ctx context.Context, e EntitlementRecord) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO nowpay_mock_entitlements
			(out_product_id, external_owner_id, external_buyer_id, billing_mode, sku_id, valid_from, valid_until)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (out_product_id, external_owner_id, external_buyer_id) DO UPDATE
		SET billing_mode=EXCLUDED.billing_mode, sku_id=EXCLUDED.sku_id`,
		e.OutProductID, e.ExternalOwnerID, e.ExternalBuyerID, e.BillingMode, e.SKUID, e.ValidFrom, e.ValidUntil)
	return err
}

func (s *PGStore) GetEntitlement(ctx context.Context, product, owner, buyer string) (*EntitlementRecord, error) {
	var e EntitlementRecord
	err := s.pool.QueryRow(ctx, `
		SELECT out_product_id, external_owner_id, external_buyer_id, billing_mode, sku_id, valid_from, valid_until
		FROM nowpay_mock_entitlements
		WHERE out_product_id=$1 AND external_owner_id=$2 AND external_buyer_id=$3`,
		product, owner, buyer).Scan(&e.OutProductID, &e.ExternalOwnerID, &e.ExternalBuyerID, &e.BillingMode, &e.SKUID, &e.ValidFrom, &e.ValidUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *PGStore) retry(fn func() error) error {
	err := fn()
	if isUnique(err) {
		err = fn()
	}
	return err
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

const chargeSelect = `
	SELECT out_product_id, external_owner_id, product_name, product_icon_url, product_url, callback_url,
		pricing_modes, capability_status, billing_mode, quota_unit, status_version, charging_options,
		configuration_url, configuration_qr_code, management_url, ticket, alipay_user_id, logon_masked, reason_message
	FROM nowpay_mock_charges`

func chargeForUpdate(ctx context.Context, tx pgx.Tx, product, owner string) (*ChargeRecord, error) {
	row := tx.QueryRow(ctx, chargeSelect+` WHERE out_product_id=$1 AND external_owner_id=$2 FOR UPDATE`, product, owner)
	rec, err := scanCharge(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func scanCharge(row pgx.Row) (*ChargeRecord, error) {
	var rec ChargeRecord
	var modes, options []byte
	err := row.Scan(
		&rec.OutProductID, &rec.ExternalOwnerID, &rec.ProductName, &rec.ProductIconURL, &rec.ProductURL, &rec.CallbackURL,
		&modes, &rec.CapabilityStatus, &rec.BillingMode, &rec.QuotaUnit, &rec.StatusVersion, &options,
		&rec.ConfigurationURL, &rec.ConfigurationQRCode, &rec.ManagementURL, &rec.Ticket, &rec.AlipayUserID, &rec.LogonMasked, &rec.ReasonMessage,
	)
	if err != nil {
		return nil, err
	}
	if len(modes) > 0 {
		if err := json.Unmarshal(modes, &rec.PricingModes); err != nil {
			return nil, err
		}
	}
	if len(options) > 0 {
		if err := json.Unmarshal(options, &rec.Options); err != nil {
			return nil, err
		}
	}
	return &rec, nil
}

func upsertCharge(ctx context.Context, tx pgx.Tx, rec *ChargeRecord) error {
	modes, err := json.Marshal(nonNilModes(rec.PricingModes))
	if err != nil {
		return err
	}
	options, err := json.Marshal(nonNilOptions(rec.Options))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO nowpay_mock_charges (
			out_product_id, external_owner_id, product_name, product_icon_url, product_url, callback_url,
			pricing_modes, capability_status, billing_mode, quota_unit, status_version, charging_options,
			configuration_url, configuration_qr_code, management_url, ticket, alipay_user_id, logon_masked, reason_message, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12::jsonb,$13,$14,$15,$16,$17,$18,$19, now())
		ON CONFLICT (out_product_id, external_owner_id) DO UPDATE SET
			product_name=EXCLUDED.product_name,
			product_icon_url=EXCLUDED.product_icon_url,
			product_url=EXCLUDED.product_url,
			callback_url=EXCLUDED.callback_url,
			pricing_modes=EXCLUDED.pricing_modes,
			capability_status=EXCLUDED.capability_status,
			billing_mode=EXCLUDED.billing_mode,
			quota_unit=EXCLUDED.quota_unit,
			status_version=EXCLUDED.status_version,
			charging_options=EXCLUDED.charging_options,
			configuration_url=EXCLUDED.configuration_url,
			configuration_qr_code=EXCLUDED.configuration_qr_code,
			management_url=EXCLUDED.management_url,
			ticket=EXCLUDED.ticket,
			alipay_user_id=EXCLUDED.alipay_user_id,
			logon_masked=EXCLUDED.logon_masked,
			reason_message=EXCLUDED.reason_message,
			updated_at=now()`,
		rec.OutProductID, rec.ExternalOwnerID, rec.ProductName, rec.ProductIconURL, rec.ProductURL, rec.CallbackURL,
		string(modes), rec.CapabilityStatus, rec.BillingMode, rec.QuotaUnit, rec.StatusVersion, string(options),
		rec.ConfigurationURL, rec.ConfigurationQRCode, rec.ManagementURL, rec.Ticket, rec.AlipayUserID, rec.LogonMasked, rec.ReasonMessage,
	)
	return err
}

func nonNilModes(in []PricingMode) []PricingMode {
	if in == nil {
		return []PricingMode{}
	}
	return in
}

const linkSelect = `
	SELECT out_request_no, fingerprint, out_product_id, external_owner_id, external_buyer_id, sku_id,
		callback_url, purchase_url, purchase_qr_code, expire_at, charging_option, paid, trade_no, paid_at, ticket
	FROM nowpay_mock_links`

func linkForUpdate(ctx context.Context, tx pgx.Tx, no string) (*LinkRecord, error) {
	row := tx.QueryRow(ctx, linkSelect+` WHERE out_request_no=$1 FOR UPDATE`, no)
	rec, err := scanLink(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return rec, err
}

func scanLink(row pgx.Row) (*LinkRecord, error) {
	var rec LinkRecord
	var option []byte
	err := row.Scan(
		&rec.OutRequestNo, &rec.Fingerprint, &rec.OutProductID, &rec.ExternalOwnerID, &rec.ExternalBuyerID, &rec.SKUID,
		&rec.CallbackURL, &rec.PurchaseURL, &rec.PurchaseQRCode, &rec.ExpireAt, &option, &rec.Paid, &rec.TradeNo, &rec.PaidAt, &rec.Ticket,
	)
	if err != nil {
		return nil, err
	}
	if len(option) > 0 && string(option) != "null" {
		var opt ChargingOption
		if err := json.Unmarshal(option, &opt); err != nil {
			return nil, err
		}
		rec.Option = &opt
	}
	return &rec, nil
}

func upsertLink(ctx context.Context, tx pgx.Tx, rec *LinkRecord) error {
	var option any
	if rec.Option != nil {
		b, err := json.Marshal(rec.Option)
		if err != nil {
			return err
		}
		option = string(b)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO nowpay_mock_links (
			out_request_no, fingerprint, out_product_id, external_owner_id, external_buyer_id, sku_id,
			callback_url, purchase_url, purchase_qr_code, expire_at, charging_option, paid, trade_no, paid_at, ticket
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13,$14,$15)
		ON CONFLICT (out_request_no) DO UPDATE SET
			fingerprint=EXCLUDED.fingerprint,
			sku_id=EXCLUDED.sku_id,
			callback_url=EXCLUDED.callback_url,
			purchase_url=EXCLUDED.purchase_url,
			purchase_qr_code=EXCLUDED.purchase_qr_code,
			expire_at=EXCLUDED.expire_at,
			charging_option=EXCLUDED.charging_option,
			paid=EXCLUDED.paid,
			trade_no=EXCLUDED.trade_no,
			paid_at=EXCLUDED.paid_at,
			ticket=EXCLUDED.ticket`,
		rec.OutRequestNo, rec.Fingerprint, rec.OutProductID, rec.ExternalOwnerID, rec.ExternalBuyerID, rec.SKUID,
		rec.CallbackURL, rec.PurchaseURL, rec.PurchaseQRCode, rec.ExpireAt, option, rec.Paid, rec.TradeNo, rec.PaidAt, rec.Ticket,
	)
	return err
}
