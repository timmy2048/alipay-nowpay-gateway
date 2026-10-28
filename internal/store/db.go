package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yin/alipay-nowpay-gateway/internal/id"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Seed(ctx context.Context, adminEmail, adminName, adminHash string) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO commission_settings (id, rate_bps, alipay_jsapi_fee_bps, alipay_nowpay_fee_bps)
		VALUES (1, 200, 60, 40)
		ON CONFLICT (id) DO NOTHING`); err != nil {
		return err
	}
	type row struct {
		mode, name, quota, period string
		cents                     int64
		active, purchasable       bool
	}
	rows := []row{
		{"ONE_TIME", "开通个人支付宝收款席位", "", "", 19900, true, true},
		{"ONE_TIME_DURATION", "时长包席位（未开放）", "", "MONTH", 0, false, false},
		{"QUANTITY", "额度包席位（未开放）", "COUNT", "", 0, false, false},
	}
	for _, r := range rows {
		var quota, period any
		if r.quota != "" {
			quota = r.quota
		}
		if r.period != "" {
			period = r.period
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO price_catalog (id, name, billing_mode, quota_unit, duration_period, amount_cents, active, purchasable)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (billing_mode) DO NOTHING`,
			id.New(), r.name, r.mode, quota, period, r.cents, r.active, r.purchasable); err != nil {
			return err
		}
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		_, err := s.pool.Exec(ctx, `INSERT INTO admins (id, email, name, password_hash) VALUES ($1,$2,$3,$4)`,
			id.New(), adminEmail, adminName, adminHash)
		return err
	}
	return nil
}

func (s *Store) AdminByEmail(ctx context.Context, email string) (Admin, error) {
	var a Admin
	err := s.pool.QueryRow(ctx, `SELECT id::text, email, name, password_hash FROM admins WHERE email=$1`, email).
		Scan(&a.ID, &a.Email, &a.Name, &a.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	return a, err
}

func (s *Store) AdminHash(ctx context.Context, adminID string) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM admins WHERE id=$1`, adminID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return hash, err
}

func (s *Store) UpdateAdminPassword(ctx context.Context, adminID, hash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE admins SET password_hash=$2 WHERE id=$1`, adminID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type PortalLogin struct {
	UserID        string
	CreatorID     string
	Email         string
	Name          string
	PasswordHash  string
	CreatorStatus string
}

func (s *Store) PortalByEmail(ctx context.Context, email string) (PortalLogin, error) {
	var p PortalLogin
	err := s.pool.QueryRow(ctx, `
		SELECT u.id::text, u.creator_id::text, u.email, u.name, u.password_hash, c.status
		FROM creator_users u JOIN creators c ON c.id = u.creator_id
		WHERE u.email=$1`, email).Scan(&p.UserID, &p.CreatorID, &p.Email, &p.Name, &p.PasswordHash, &p.CreatorStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return PortalLogin{}, ErrNotFound
	}
	return p, err
}

func (s *Store) CreateSession(ctx context.Context, hash, actorType, actorID, creatorID string, exp time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, actor_type, actor_id, creator_id, expires_at)
		VALUES ($1,$2,$3,$4,$5)`, hash, actorType, actorID, uuidOrNil(creatorID), exp)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash)
	return err
}

func (s *Store) LookupSession(ctx context.Context, hash string) (Actor, error) {
	var a Actor
	var creator sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT s.actor_type, s.actor_id::text, s.creator_id::text, s.expires_at,
			COALESCE(ad.email, u.email, ''), COALESCE(ad.name, u.name, ''), COALESCE(c.status, '')
		FROM sessions s
		LEFT JOIN admins ad ON s.actor_type = 'admin' AND ad.id = s.actor_id
		LEFT JOIN creator_users u ON s.actor_type = 'creator' AND u.id = s.actor_id
		LEFT JOIN creators c ON c.id = s.creator_id
		WHERE s.token_hash=$1`, hash).Scan(&a.Type, &a.ID, &creator, &a.ExpiresAt, &a.Email, &a.Name, &a.CreatorStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Actor{}, ErrNotFound
	}
	if err != nil {
		return Actor{}, err
	}
	if creator.Valid {
		a.CreatorID = creator.String
	}
	if time.Now().After(a.ExpiresAt) {
		_ = s.DeleteSession(ctx, hash)
		return Actor{}, ErrNotFound
	}
	return a, nil
}

func (s *Store) CreateCreator(ctx context.Context, c Creator, userID, portalEmail, portalName, portalHash string) (Creator, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Creator{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
		INSERT INTO creators (id, public_code, name, contact_email, phone, notes)
		VALUES ($1,$2,$3,$4,$5,$6)`, c.ID, c.PublicCode, c.Name, c.ContactEmail, c.Phone, c.Notes)
	if err != nil {
		return Creator{}, err
	}
	if portalEmail != "" {
		_, err = tx.Exec(ctx, `
			INSERT INTO creator_users (id, creator_id, email, name, password_hash)
			VALUES ($1,$2,$3,$4,$5)`, userID, c.ID, portalEmail, portalName, portalHash)
		if err != nil {
			return Creator{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Creator{}, err
	}
	return s.GetCreator(ctx, c.ID)
}

func (s *Store) ListCreators(ctx context.Context) ([]Creator, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id::text, c.public_code, c.name, c.contact_email, c.phone, c.status, c.notes,
			COALESCE(u.id::text, ''), COALESCE(u.email, ''), COALESCE(u.name, ''), c.created_at
		FROM creators c
		LEFT JOIN creator_users u ON u.creator_id = c.id
		ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Creator
	for rows.Next() {
		var c Creator
		if err := rows.Scan(&c.ID, &c.PublicCode, &c.Name, &c.ContactEmail, &c.Phone, &c.Status, &c.Notes, &c.PortalUserID, &c.PortalEmail, &c.PortalName, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []Creator{}
	}
	return out, rows.Err()
}

func (s *Store) GetCreator(ctx context.Context, creatorID string) (Creator, error) {
	var c Creator
	err := s.pool.QueryRow(ctx, `
		SELECT c.id::text, c.public_code, c.name, c.contact_email, c.phone, c.status, c.notes,
			COALESCE(u.id::text, ''), COALESCE(u.email, ''), COALESCE(u.name, ''), c.created_at
		FROM creators c LEFT JOIN creator_users u ON u.creator_id = c.id
		WHERE c.id=$1`, creatorID).Scan(&c.ID, &c.PublicCode, &c.Name, &c.ContactEmail, &c.Phone, &c.Status, &c.Notes, &c.PortalUserID, &c.PortalEmail, &c.PortalName, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Creator{}, ErrNotFound
	}
	return c, err
}

func (s *Store) UpdateCreator(ctx context.Context, c Creator) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE creators SET name=$2, contact_email=$3, phone=$4, status=$5, notes=$6, updated_at=now()
		WHERE id=$1`, c.ID, c.Name, c.ContactEmail, c.Phone, c.Status, c.Notes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetCreatorPassword(ctx context.Context, creatorID, hash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE creator_users SET password_hash=$2 WHERE creator_id=$1`, creatorID, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const projectSelect = `
	SELECT p.id::text, p.public_code, p.creator_id::text, c.name, c.public_code,
		p.name, p.status, p.callback_url, p.notify_url, p.billing_mode,
		p.seat_price_cents, p.customer_price_cents, p.commission_bps,
		COALESCE(p.seat_price_cents, cat.amount_cents),
		COALESCE(p.commission_bps, com.rate_bps),
		(SELECT count(*)::int FROM seats s WHERE s.project_id = p.id),
		(SELECT count(*)::int FROM seats s WHERE s.project_id = p.id AND s.status = 'bound'),
		p.created_at, p.updated_at
	FROM projects p
	JOIN creators c ON c.id = p.creator_id
	JOIN price_catalog cat ON cat.billing_mode = 'ONE_TIME'
	JOIN commission_settings com ON com.id = 1`

func scanProject(row pgx.Row) (Project, error) {
	var p Project
	var seat sql.NullInt64
	var bps sql.NullInt32
	err := row.Scan(
		&p.ID, &p.PublicCode, &p.CreatorID, &p.CreatorName, &p.CreatorCode,
		&p.Name, &p.Status, &p.CallbackURL, &p.NotifyURL, &p.BillingMode,
		&seat, &p.CustomerPriceCents, &bps,
		&p.EffectiveSeatPriceCents, &p.EffectiveCommissionBPS,
		&p.SeatCount, &p.BoundCount, &p.CreatedAt, &p.UpdatedAt,
	)
	if seat.Valid {
		v := seat.Int64
		p.SeatPriceCents = &v
	}
	if bps.Valid {
		v := int(bps.Int32)
		p.CommissionBPS = &v
	}
	return p, err
}

func (s *Store) CreateProject(ctx context.Context, p Project) (Project, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO projects (
			id, public_code, creator_id, name, status, callback_url, notify_url, billing_mode,
			seat_price_cents, customer_price_cents, commission_bps
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		p.ID, p.PublicCode, p.CreatorID, p.Name, p.Status, p.CallbackURL, p.NotifyURL, p.BillingMode,
		p.SeatPriceCents, p.CustomerPriceCents, p.CommissionBPS)
	if err != nil {
		return Project{}, err
	}
	return s.GetProject(ctx, p.ID)
}

func (s *Store) UpdateProject(ctx context.Context, p Project) (Project, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE projects SET name=$2, status=$3, callback_url=$4, notify_url=$5, billing_mode=$6,
			seat_price_cents=$7, customer_price_cents=$8, commission_bps=$9, updated_at=now()
		WHERE id=$1`,
		p.ID, p.Name, p.Status, p.CallbackURL, p.NotifyURL, p.BillingMode,
		p.SeatPriceCents, p.CustomerPriceCents, p.CommissionBPS)
	if err != nil {
		return Project{}, err
	}
	if tag.RowsAffected() == 0 {
		return Project{}, ErrNotFound
	}
	return s.GetProject(ctx, p.ID)
}

func (s *Store) UpdateEndpoints(ctx context.Context, projectID, callback, notify string) (Project, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE projects SET callback_url=$2, notify_url=$3, updated_at=now() WHERE id=$1`,
		projectID, callback, notify)
	if err != nil {
		return Project{}, err
	}
	if tag.RowsAffected() == 0 {
		return Project{}, ErrNotFound
	}
	return s.GetProject(ctx, projectID)
}

func (s *Store) GetProject(ctx context.Context, projectID string) (Project, error) {
	p, err := scanProject(s.pool.QueryRow(ctx, projectSelect+` WHERE p.id=$1`, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

func (s *Store) ListProjects(ctx context.Context, creatorID string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, projectSelect+` WHERE ($1 = '' OR p.creator_id::text = $1) ORDER BY p.created_at DESC`, creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []Project{}
	}
	return out, rows.Err()
}

func (s *Store) ListCatalog(ctx context.Context) ([]PriceItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, name, billing_mode, COALESCE(quota_unit, ''), COALESCE(duration_period, ''),
			amount_cents, currency, active, purchasable, updated_at
		FROM price_catalog ORDER BY CASE billing_mode WHEN 'ONE_TIME' THEN 0 WHEN 'ONE_TIME_DURATION' THEN 1 ELSE 2 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriceItem
	for rows.Next() {
		var it PriceItem
		if err := rows.Scan(&it.ID, &it.Name, &it.BillingMode, &it.QuotaUnit, &it.DurationPeriod, &it.AmountCents, &it.Currency, &it.Active, &it.Purchasable, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	if out == nil {
		out = []PriceItem{}
	}
	return out, rows.Err()
}

func (s *Store) UpdateOneTimePrice(ctx context.Context, name string, cents int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE price_catalog SET name=$1, amount_cents=$2, active=true, purchasable=true, updated_at=now()
		WHERE billing_mode='ONE_TIME'`, name, cents)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Commission(ctx context.Context) (CommissionSettings, error) {
	var c CommissionSettings
	err := s.pool.QueryRow(ctx, `
		SELECT rate_bps, alipay_jsapi_fee_bps, alipay_nowpay_fee_bps, updated_at
		FROM commission_settings WHERE id=1`).Scan(&c.RateBPS, &c.AlipayJSAPIBPS, &c.AlipayNowPayBPS, &c.UpdatedAt)
	return c, err
}

func (s *Store) UpdateCommission(ctx context.Context, c CommissionSettings) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE commission_settings SET rate_bps=$1, alipay_jsapi_fee_bps=$2, alipay_nowpay_fee_bps=$3, updated_at=now()
		WHERE id=1`, c.RateBPS, c.AlipayJSAPIBPS, c.AlipayNowPayBPS)
	return err
}

func (s *Store) CreateSeat(ctx context.Context, seat Seat, channel string) (Seat, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Seat{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM projects WHERE id=$1 FOR UPDATE`, seat.ProjectID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Seat{}, ErrNotFound
	}
	if err != nil {
		return Seat{}, err
	}
	if status != "active" {
		return Seat{}, ErrInactive
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(ordinal),0)+1 FROM seats WHERE project_id=$1`, seat.ProjectID).Scan(&seat.Ordinal); err != nil {
		return Seat{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO seats (id, public_code, project_id, creator_id, ordinal, status, billing_mode, amount_cents, currency)
		VALUES ($1,$2,$3,$4,$5,'pending_payment',$6,$7,'CNY')`,
		seat.ID, seat.PublicCode, seat.ProjectID, seat.CreatorID, seat.Ordinal, seat.BillingMode, seat.AmountCents)
	if err != nil {
		return Seat{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO buyout_orders (id, seat_id, creator_id, project_id, billing_mode, amount_cents, currency, status, channel)
		VALUES ($1,$2,$3,$4,$5,$6,'CNY','pending',$7)`,
		id.New(), seat.ID, seat.CreatorID, seat.ProjectID, seat.BillingMode, seat.AmountCents, channel)
	if err != nil {
		return Seat{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Seat{}, err
	}
	return s.GetSeat(ctx, seat.ID)
}

func (s *Store) MarkSeatPaid(ctx context.Context, seatID, channel, ref string) (Seat, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Seat{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM seats WHERE id=$1 FOR UPDATE`, seatID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Seat{}, ErrNotFound
	}
	if err != nil {
		return Seat{}, err
	}
	if status == "pending_payment" {
		if _, err := tx.Exec(ctx, `
			UPDATE buyout_orders SET status='paid', channel=$2, payment_ref=$3, paid_at=now()
			WHERE seat_id=$1`, seatID, channel, ref); err != nil {
			return Seat{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE seats SET status='paid_unbound', updated_at=now() WHERE id=$1`, seatID); err != nil {
			return Seat{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Seat{}, err
	}
	return s.GetSeat(ctx, seatID)
}

func (s *Store) SaveCapability(ctx context.Context, seatID string, cap Capability) error {
	options := cap.Options
	if len(options) == 0 {
		options = []byte("[]")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE seats SET status=$2, capability_status=$3, alipay_user_id=$4, alipay_logon_masked=$5,
			configuration_url=$6, configuration_qr_code=$7, management_url=$8, billing_mode_actual=$9,
			charging_options=$10::jsonb, status_version=$11,
			bound_at=CASE WHEN $12 THEN COALESCE(bound_at, now()) ELSE bound_at END,
			updated_at=now()
		WHERE id=$1`,
		seatID, cap.SeatStatus, cap.CapabilityStatus, cap.AlipayUserID, cap.LogonMasked,
		cap.ConfigurationURL, cap.ConfigurationQRCode, cap.ManagementURL, cap.BillingModeActual,
		string(options), cap.StatusVersion, cap.MarkBound)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const seatSelect = `
	SELECT s.id::text, s.public_code, s.project_id::text, s.creator_id::text,
		COALESCE(p.name, ''), COALESCE(c.name, ''),
		s.ordinal, s.status, s.billing_mode, s.amount_cents, s.currency,
		s.capability_status, s.alipay_user_id, s.alipay_logon_masked,
		s.configuration_url, s.configuration_qr_code, s.management_url,
		s.billing_mode_actual, s.charging_options, s.status_version, s.bound_at, s.created_at
	FROM seats s
	LEFT JOIN projects p ON p.id = s.project_id
	LEFT JOIN creators c ON c.id = s.creator_id`

func scanSeat(row pgx.Row) (Seat, error) {
	var seat Seat
	var options []byte
	err := row.Scan(
		&seat.ID, &seat.PublicCode, &seat.ProjectID, &seat.CreatorID, &seat.ProjectName, &seat.CreatorName,
		&seat.Ordinal, &seat.Status, &seat.BillingMode, &seat.AmountCents, &seat.Currency,
		&seat.CapabilityStatus, &seat.AlipayUserID, &seat.AlipayLogonMasked,
		&seat.ConfigurationURL, &seat.ConfigurationQRCode, &seat.ManagementURL,
		&seat.BillingModeActual, &options, &seat.StatusVersion, &seat.BoundAt, &seat.CreatedAt,
	)
	if len(options) == 0 {
		options = []byte("[]")
	}
	seat.ChargingOptions = options
	return seat, err
}

func (s *Store) GetSeat(ctx context.Context, seatID string) (Seat, error) {
	seat, err := scanSeat(s.pool.QueryRow(ctx, seatSelect+` WHERE s.id=$1`, seatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Seat{}, ErrNotFound
	}
	return seat, err
}

func (s *Store) SeatByCode(ctx context.Context, code string) (Seat, error) {
	seat, err := scanSeat(s.pool.QueryRow(ctx, seatSelect+` WHERE s.public_code=$1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return Seat{}, ErrNotFound
	}
	return seat, err
}

func (s *Store) ListSeats(ctx context.Context, projectID, creatorID string) ([]Seat, error) {
	rows, err := s.pool.Query(ctx, seatSelect+`
		WHERE ($1 = '' OR s.project_id::text = $1) AND ($2 = '' OR s.creator_id::text = $2)
		ORDER BY s.created_at DESC`, projectID, creatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Seat
	for rows.Next() {
		seat, err := scanSeat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, seat)
	}
	if out == nil {
		out = []Seat{}
	}
	return out, rows.Err()
}

func (s *Store) InsertOpenPurchase(ctx context.Context, p Purchase) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO purchases (
			id, seat_id, project_id, creator_id, out_request_no, external_buyer_id, sku_id,
			gross_cents, currency, status, purchase_url, purchase_qr_code, expire_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'CNY','link_created',$9,$10,$11)
		ON CONFLICT (out_request_no) DO NOTHING`,
		p.ID, p.SeatID, p.ProjectID, p.CreatorID, p.OutRequestNo, p.ExternalBuyerID, p.SKUID,
		p.GrossCents, p.PurchaseURL, p.PurchaseQRCode, p.ExpireAt)
	return err
}

func (s *Store) BookPaid(ctx context.Context, p Purchase, lines []LedgerLine) (Purchase, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Purchase{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existingID, status string
	err = tx.QueryRow(ctx, `SELECT id::text, status FROM purchases WHERE out_request_no=$1 FOR UPDATE`, p.OutRequestNo).Scan(&existingID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		if p.ID == "" {
			p.ID = id.New()
		}
		p.Status = "paid"
		_, err = tx.Exec(ctx, `
			INSERT INTO purchases (
				id, seat_id, project_id, creator_id, out_request_no, external_buyer_id, sku_id,
				gross_cents, status, decision, alipay_trade_no, commission_cents, commission_bps,
				alipay_fee_cents, creator_net_cents, estimates_exceed_gross, purchase_url, purchase_qr_code,
				expire_at, paid_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'paid',$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
			p.ID, p.SeatID, p.ProjectID, p.CreatorID, p.OutRequestNo, p.ExternalBuyerID, p.SKUID,
			p.GrossCents, p.Decision, p.AlipayTradeNo, p.CommissionCents, p.CommissionBPS,
			p.AlipayFeeCents, p.CreatorNetCents, p.EstimatesExceedGross, p.PurchaseURL, p.PurchaseQRCode,
			p.ExpireAt, p.PaidAt)
		if err != nil {
			return Purchase{}, false, err
		}
		if err := insertLines(ctx, tx, p, lines); err != nil {
			return Purchase{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Purchase{}, false, err
		}
		loaded, err := s.PurchaseByRequest(ctx, p.OutRequestNo)
		return loaded, true, err
	}
	if err != nil {
		return Purchase{}, false, err
	}
	if status == "paid" {
		if err := tx.Commit(ctx); err != nil {
			return Purchase{}, false, err
		}
		loaded, err := s.PurchaseByRequest(ctx, p.OutRequestNo)
		return loaded, false, err
	}
	p.ID = existingID
	p.Status = "paid"
	_, err = tx.Exec(ctx, `
		UPDATE purchases SET status='paid', gross_cents=$2, decision=$3, alipay_trade_no=$4,
			commission_cents=$5, commission_bps=$6, alipay_fee_cents=$7, creator_net_cents=$8,
			estimates_exceed_gross=$9, sku_id=$10, paid_at=$11
		WHERE id=$1`,
		p.ID, p.GrossCents, p.Decision, p.AlipayTradeNo, p.CommissionCents, p.CommissionBPS,
		p.AlipayFeeCents, p.CreatorNetCents, p.EstimatesExceedGross, p.SKUID, p.PaidAt)
	if err != nil {
		return Purchase{}, false, err
	}
	if err := insertLines(ctx, tx, p, lines); err != nil {
		return Purchase{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Purchase{}, false, err
	}
	loaded, err := s.PurchaseByRequest(ctx, p.OutRequestNo)
	return loaded, true, err
}

func insertLines(ctx context.Context, tx pgx.Tx, p Purchase, lines []LedgerLine) error {
	for _, ln := range lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (id, purchase_id, seat_id, project_id, creator_id, account, direction, amount_cents, status, memo)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			id.New(), p.ID, p.SeatID, p.ProjectID, p.CreatorID, ln.Account, ln.Direction, ln.Amount, ln.Status, ln.Memo); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SetDecision(ctx context.Context, requestNo, decision string) error {
	_, err := s.pool.Exec(ctx, `UPDATE purchases SET decision=$2 WHERE out_request_no=$1`, requestNo, decision)
	return err
}

const purchaseSelect = `
	SELECT id::text, seat_id::text, project_id::text, creator_id::text, out_request_no, external_buyer_id, sku_id,
		gross_cents, currency, status, decision, alipay_trade_no, commission_cents, commission_bps,
		alipay_fee_cents, creator_net_cents, estimates_exceed_gross, purchase_url, purchase_qr_code,
		expire_at, paid_at, created_at
	FROM purchases`

func scanPurchase(row pgx.Row) (Purchase, error) {
	var p Purchase
	err := row.Scan(
		&p.ID, &p.SeatID, &p.ProjectID, &p.CreatorID, &p.OutRequestNo, &p.ExternalBuyerID, &p.SKUID,
		&p.GrossCents, &p.Currency, &p.Status, &p.Decision, &p.AlipayTradeNo, &p.CommissionCents, &p.CommissionBPS,
		&p.AlipayFeeCents, &p.CreatorNetCents, &p.EstimatesExceedGross, &p.PurchaseURL, &p.PurchaseQRCode,
		&p.ExpireAt, &p.PaidAt, &p.CreatedAt,
	)
	return p, err
}

func (s *Store) PurchaseByRequest(ctx context.Context, requestNo string) (Purchase, error) {
	p, err := scanPurchase(s.pool.QueryRow(ctx, purchaseSelect+` WHERE out_request_no=$1`, requestNo))
	if errors.Is(err, pgx.ErrNoRows) {
		return Purchase{}, ErrNotFound
	}
	return p, err
}

func (s *Store) ListPurchases(ctx context.Context, projectID, creatorID string, limit int) ([]Purchase, error) {
	rows, err := s.pool.Query(ctx, purchaseSelect+`
		WHERE ($1 = '' OR project_id::text = $1) AND ($2 = '' OR creator_id::text = $2)
		ORDER BY created_at DESC LIMIT $3`, projectID, creatorID, clamp(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Purchase
	for rows.Next() {
		p, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []Purchase{}
	}
	return out, rows.Err()
}

func (s *Store) ListLedger(ctx context.Context, limit int) ([]LedgerEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, COALESCE(purchase_id::text, ''), COALESCE(seat_id::text, ''),
			COALESCE(project_id::text, ''), COALESCE(creator_id::text, ''), COALESCE(settlement_id::text, ''),
			account, direction, amount_cents, status, memo, created_at
		FROM ledger_entries ORDER BY created_at DESC LIMIT $1`, clamp(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ID, &e.PurchaseID, &e.SeatID, &e.ProjectID, &e.CreatorID, &e.SettlementID, &e.Account, &e.Direction, &e.AmountCents, &e.Status, &e.Memo, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if out == nil {
		out = []LedgerEntry{}
	}
	return out, rows.Err()
}

func (s *Store) Settle(ctx context.Context, note, engine string) (Settlement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Settlement{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `
		SELECT amount_cents FROM ledger_entries
		WHERE account='platform_commission_receivable' AND direction='debit' AND status='booked'
		FOR UPDATE`)
	if err != nil {
		return Settlement{}, err
	}
	var total int64
	var count int
	for rows.Next() {
		var amt int64
		if err := rows.Scan(&amt); err != nil {
			rows.Close()
			return Settlement{}, err
		}
		total += amt
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Settlement{}, err
	}
	if count == 0 {
		return Settlement{}, ErrNothingToSettle
	}
	batch := Settlement{ID: id.New(), TotalCents: total, EntryCount: count, Note: note, Engine: engine, SettledAt: time.Now()}
	if _, err := tx.Exec(ctx, `
		INSERT INTO settlement_batches (id, total_cents, entry_count, note, engine, settled_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, batch.ID, batch.TotalCents, batch.EntryCount, batch.Note, batch.Engine, batch.SettledAt); err != nil {
		return Settlement{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ledger_entries SET status='settled', settlement_id=$1
		WHERE status='booked' AND account IN ('platform_commission_receivable', 'platform_commission_income')`, batch.ID); err != nil {
		return Settlement{}, err
	}
	for _, ln := range []LedgerLine{
		{Account: "platform_cash", Direction: "debit", Amount: total, Status: "settled", Memo: "模拟结算，平台收到已入账抽成"},
		{Account: "platform_commission_receivable", Direction: "credit", Amount: total, Status: "settled", Memo: "结清抽成应收"},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (id, settlement_id, account, direction, amount_cents, status, memo)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, id.New(), batch.ID, ln.Account, ln.Direction, ln.Amount, ln.Status, ln.Memo); err != nil {
			return Settlement{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Settlement{}, err
	}
	return batch, nil
}

func (s *Store) Dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*)::int FROM creators),
			(SELECT count(*)::int FROM projects),
			(SELECT count(*)::int FROM seats WHERE status='bound'),
			(SELECT count(*)::int FROM seats WHERE status='pending_payment'),
			(SELECT count(*)::int FROM seats WHERE status IN ('paid_unbound','configuring')),
			(SELECT count(*)::int FROM purchases WHERE status='paid'),
			(SELECT COALESCE(SUM(gross_cents),0) FROM purchases WHERE status='paid'),
			(SELECT COALESCE(SUM(amount_cents),0) FROM ledger_entries WHERE account='platform_commission_receivable' AND direction='debit' AND status='booked'),
			(SELECT COALESCE(SUM(amount_cents),0) FROM ledger_entries WHERE account='platform_cash' AND direction='debit')`).Scan(
		&d.CreatorCount, &d.ProjectCount, &d.BoundSeats, &d.UnpaidSeats, &d.ConfiguringSeats,
		&d.PaidPurchaseCount, &d.GrossCents, &d.CommissionBookedCents, &d.CommissionSettledCents)
	if err != nil {
		return Dashboard{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT s.id::text, s.ordinal, s.status, s.amount_cents, p.id::text, p.name, c.name
		FROM seats s
		JOIN projects p ON p.id = s.project_id
		JOIN creators c ON c.id = s.creator_id
		WHERE s.status IN ('pending_payment', 'paid_unbound', 'configuring')
		ORDER BY s.created_at ASC LIMIT 12`)
	if err != nil {
		return Dashboard{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var n Need
		if err := rows.Scan(&n.SeatID, &n.Ordinal, &n.Status, &n.AmountCents, &n.ProjectID, &n.ProjectName, &n.CreatorName); err != nil {
			return Dashboard{}, err
		}
		d.Needs = append(d.Needs, n)
	}
	if d.Needs == nil {
		d.Needs = []Need{}
	}
	d.Recent, err = s.ListPurchases(ctx, "", "", 8)
	return d, err
}

func (s *Store) WriteAudit(ctx context.Context, a Audit) error {
	detail := a.Detail
	if len(detail) == 0 {
		detail = []byte(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, resource_type, resource_id, detail, ip)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7)`,
		a.ActorType, uuidOrNil(a.ActorID), a.Action, a.ResourceType, a.ResourceID, string(detail), a.IP)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]Audit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, actor_type, COALESCE(actor_id::text, ''), action, resource_type, resource_id, detail, ip, created_at
		FROM audit_logs ORDER BY id DESC LIMIT $1`, clamp(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Audit
	for rows.Next() {
		var a Audit
		var detail []byte
		if err := rows.Scan(&a.ID, &a.ActorType, &a.ActorID, &a.Action, &a.ResourceType, &a.ResourceID, &detail, &a.IP, &a.CreatedAt); err != nil {
			return nil, err
		}
		if len(detail) == 0 {
			detail = []byte(`{}`)
		}
		a.Detail = detail
		out = append(out, a)
	}
	if out == nil {
		out = []Audit{}
	}
	return out, rows.Err()
}

func (s *Store) InsertWebhook(ctx context.Context, w Webhook) error {
	payload := w.Payload
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, project_id, event_type, target_url, payload, status, attempts, last_error)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8)`,
		w.ID, uuidOrNil(w.ProjectID), w.EventType, w.TargetURL, string(payload), w.Status, w.Attempts, w.LastError)
	return err
}

func (s *Store) FinishWebhook(ctx context.Context, id, status, last string, attempts int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE webhook_deliveries SET status=$2, last_error=$3, attempts=$4, updated_at=now() WHERE id=$1`,
		id, status, last, attempts)
	return err
}

func (s *Store) ListWebhooks(ctx context.Context, limit int) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, COALESCE(project_id::text, ''), event_type, target_url, payload, status, attempts, last_error, created_at
		FROM webhook_deliveries ORDER BY created_at DESC LIMIT $1`, clamp(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var w Webhook
		var payload []byte
		if err := rows.Scan(&w.ID, &w.ProjectID, &w.EventType, &w.TargetURL, &payload, &w.Status, &w.Attempts, &w.LastError, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.Payload = payload
		out = append(out, w)
	}
	if out == nil {
		out = []Webhook{}
	}
	return out, rows.Err()
}

func uuidOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func clamp(n int) int {
	if n <= 0 || n > 200 {
		return 50
	}
	return n
}

// keep json import used if a caller passes raw messages through this file
var _ = json.RawMessage(nil)
