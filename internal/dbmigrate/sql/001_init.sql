CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE admins (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE creators (
    id uuid PRIMARY KEY,
    public_code text NOT NULL UNIQUE,
    name text NOT NULL,
    contact_email text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE creator_users (
    id uuid PRIMARY KEY,
    creator_id uuid NOT NULL UNIQUE REFERENCES creators (id),
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    actor_type text NOT NULL CHECK (actor_type IN ('admin', 'creator')),
    actor_id uuid NOT NULL,
    creator_id uuid,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE price_catalog (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    billing_mode text NOT NULL UNIQUE CHECK (billing_mode IN ('ONE_TIME', 'ONE_TIME_DURATION', 'QUANTITY')),
    quota_unit text,
    duration_period text,
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    currency text NOT NULL DEFAULT 'CNY',
    active boolean NOT NULL DEFAULT false,
    purchasable boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE commission_settings (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    rate_bps integer NOT NULL CHECK (rate_bps BETWEEN 0 AND 10000),
    alipay_jsapi_fee_bps integer NOT NULL CHECK (alipay_jsapi_fee_bps BETWEEN 0 AND 10000),
    alipay_nowpay_fee_bps integer NOT NULL CHECK (alipay_nowpay_fee_bps BETWEEN 0 AND 10000),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id uuid PRIMARY KEY,
    public_code text NOT NULL UNIQUE,
    creator_id uuid NOT NULL REFERENCES creators (id),
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    callback_url text NOT NULL DEFAULT '',
    notify_url text NOT NULL DEFAULT '',
    billing_mode text NOT NULL DEFAULT 'ONE_TIME' CHECK (billing_mode IN ('ONE_TIME', 'ONE_TIME_DURATION', 'QUANTITY')),
    seat_price_cents bigint CHECK (seat_price_cents IS NULL OR seat_price_cents >= 0),
    customer_price_cents bigint NOT NULL CHECK (customer_price_cents >= 1),
    commission_bps integer CHECK (commission_bps IS NULL OR commission_bps BETWEEN 0 AND 10000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE seats (
    id uuid PRIMARY KEY,
    public_code text NOT NULL UNIQUE,
    project_id uuid NOT NULL REFERENCES projects (id),
    creator_id uuid NOT NULL REFERENCES creators (id),
    ordinal integer NOT NULL CHECK (ordinal >= 1),
    status text NOT NULL CHECK (status IN ('pending_payment', 'paid_unbound', 'configuring', 'bound', 'disabled')),
    billing_mode text NOT NULL,
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    currency text NOT NULL DEFAULT 'CNY',
    capability_status text NOT NULL DEFAULT 'NOT_CONFIGURED',
    alipay_user_id text NOT NULL DEFAULT '',
    alipay_logon_masked text NOT NULL DEFAULT '',
    configuration_url text NOT NULL DEFAULT '',
    configuration_qr_code text NOT NULL DEFAULT '',
    management_url text NOT NULL DEFAULT '',
    billing_mode_actual text NOT NULL DEFAULT '',
    charging_options jsonb NOT NULL DEFAULT '[]'::jsonb,
    status_version integer NOT NULL DEFAULT 0,
    bound_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, ordinal)
);

CREATE TABLE buyout_orders (
    id uuid PRIMARY KEY,
    seat_id uuid NOT NULL UNIQUE REFERENCES seats (id),
    creator_id uuid NOT NULL REFERENCES creators (id),
    project_id uuid NOT NULL REFERENCES projects (id),
    billing_mode text NOT NULL,
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    currency text NOT NULL DEFAULT 'CNY',
    status text NOT NULL CHECK (status IN ('pending', 'paid', 'cancelled')),
    channel text NOT NULL CHECK (channel IN ('mock_checkout', 'admin_recorded')),
    payment_ref text NOT NULL DEFAULT '',
    paid_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE purchases (
    id uuid PRIMARY KEY,
    seat_id uuid NOT NULL REFERENCES seats (id),
    project_id uuid NOT NULL REFERENCES projects (id),
    creator_id uuid NOT NULL REFERENCES creators (id),
    out_request_no text NOT NULL UNIQUE,
    external_buyer_id text NOT NULL,
    sku_id text NOT NULL DEFAULT '',
    gross_cents bigint NOT NULL CHECK (gross_cents >= 0),
    currency text NOT NULL DEFAULT 'CNY',
    status text NOT NULL CHECK (status IN ('link_created', 'paid', 'closed')),
    decision text NOT NULL DEFAULT '',
    alipay_trade_no text NOT NULL DEFAULT '',
    commission_cents bigint NOT NULL DEFAULT 0,
    commission_bps integer NOT NULL DEFAULT 0,
    alipay_fee_cents bigint NOT NULL DEFAULT 0,
    creator_net_cents bigint NOT NULL DEFAULT 0,
    estimates_exceed_gross boolean NOT NULL DEFAULT false,
    purchase_url text NOT NULL DEFAULT '',
    purchase_qr_code text NOT NULL DEFAULT '',
    expire_at timestamptz,
    paid_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX purchases_project_idx ON purchases (project_id, created_at DESC);

CREATE TABLE ledger_entries (
    id uuid PRIMARY KEY,
    purchase_id uuid,
    seat_id uuid,
    project_id uuid,
    creator_id uuid,
    settlement_id uuid,
    account text NOT NULL,
    direction text NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    status text NOT NULL CHECK (status IN ('booked', 'settled', 'memo')),
    memo text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ledger_open_idx ON ledger_entries (account, status);

CREATE TABLE settlement_batches (
    id uuid PRIMARY KEY,
    total_cents bigint NOT NULL CHECK (total_cents >= 0),
    entry_count integer NOT NULL,
    note text NOT NULL DEFAULT '',
    engine text NOT NULL,
    settled_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id bigserial PRIMARY KEY,
    actor_type text NOT NULL,
    actor_id uuid,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text NOT NULL DEFAULT '',
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_created_idx ON audit_logs (created_at DESC);

CREATE TABLE webhook_deliveries (
    id uuid PRIMARY KEY,
    project_id uuid,
    event_type text NOT NULL,
    target_url text NOT NULL DEFAULT '',
    payload jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'delivered', 'skipped', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE nowpay_mock_charges (
    out_product_id text NOT NULL,
    external_owner_id text NOT NULL,
    product_name text NOT NULL DEFAULT '',
    product_icon_url text NOT NULL DEFAULT '',
    product_url text NOT NULL DEFAULT '',
    callback_url text NOT NULL DEFAULT '',
    pricing_modes jsonb NOT NULL DEFAULT '[]'::jsonb,
    capability_status text NOT NULL,
    billing_mode text NOT NULL DEFAULT '',
    quota_unit text NOT NULL DEFAULT '',
    status_version integer NOT NULL DEFAULT 0,
    charging_options jsonb NOT NULL DEFAULT '[]'::jsonb,
    configuration_url text NOT NULL DEFAULT '',
    configuration_qr_code text NOT NULL DEFAULT '',
    management_url text NOT NULL DEFAULT '',
    ticket text NOT NULL DEFAULT '',
    alipay_user_id text NOT NULL DEFAULT '',
    logon_masked text NOT NULL DEFAULT '',
    reason_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (out_product_id, external_owner_id)
);

CREATE UNIQUE INDEX nowpay_mock_charges_ticket_idx ON nowpay_mock_charges (ticket) WHERE ticket <> '';

CREATE TABLE nowpay_mock_links (
    out_request_no text PRIMARY KEY,
    fingerprint text NOT NULL,
    out_product_id text NOT NULL,
    external_owner_id text NOT NULL,
    external_buyer_id text NOT NULL,
    sku_id text NOT NULL DEFAULT '',
    callback_url text NOT NULL DEFAULT '',
    purchase_url text NOT NULL DEFAULT '',
    purchase_qr_code text NOT NULL DEFAULT '',
    expire_at timestamptz NOT NULL,
    charging_option jsonb,
    paid boolean NOT NULL DEFAULT false,
    trade_no text NOT NULL DEFAULT '',
    paid_at timestamptz,
    ticket text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE nowpay_mock_entitlements (
    out_product_id text NOT NULL,
    external_owner_id text NOT NULL,
    external_buyer_id text NOT NULL,
    billing_mode text NOT NULL,
    sku_id text NOT NULL DEFAULT '',
    valid_from timestamptz,
    valid_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (out_product_id, external_owner_id, external_buyer_id)
);
