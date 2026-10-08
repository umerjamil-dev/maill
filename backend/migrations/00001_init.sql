-- +goose Up

-- Tenants: one rental account per customer company.
CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    plan_id     UUID,
    status      TEXT NOT NULL DEFAULT 'active', -- active | suspended
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users: login identities, attached to a tenant.
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email       TEXT NOT NULL UNIQUE,
    password    TEXT NOT NULL, -- bcrypt hash
    full_name   TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL DEFAULT 'member', -- owner | admin | member
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_tenant ON users(tenant_id);

-- API keys for programmatic sending (hashed storage).
CREATE TABLE api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL UNIQUE,
    last_used_at TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);

-- SMTP accounts (customer BYO). Credentials encrypted at rest.
CREATE TABLE smtp_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    host            TEXT NOT NULL,
    port            INT  NOT NULL DEFAULT 587,
    username        TEXT NOT NULL,
    password_enc    BYTEA NOT NULL,        -- AES-256-GCM
    from_email      TEXT NOT NULL,
    from_name       TEXT NOT NULL DEFAULT '',
    auth_method     TEXT NOT NULL DEFAULT 'login', -- login | plain | cram-md5 | none
    tls_mode        TEXT NOT NULL DEFAULT 'starttls', -- starttls | ssl | none
    max_emails_per_min INT NOT NULL DEFAULT 60,       -- throttle per provider
    max_concurrency INT NOT NULL DEFAULT 2,
    rotation_weight INT NOT NULL DEFAULT 1,
    status          TEXT NOT NULL DEFAULT 'active',
    verified        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_smtp_tenant ON smtp_accounts(tenant_id);

-- Sending domains with SPF/DKIM/DMARC verification state.
CREATE TABLE sending_domains (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain      TEXT NOT NULL UNIQUE,
    dkim_selector TEXT NOT NULL DEFAULT 'default',
    dkim_private_key_enc BYTEA,
    spf_verified     BOOLEAN NOT NULL DEFAULT FALSE,
    dkim_verified    BOOLEAN NOT NULL DEFAULT FALSE,
    dmarc_verified   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sending_domains_tenant ON sending_domains(tenant_id);

-- Subscriber lists.
CREATE TABLE lists (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_lists_tenant ON lists(tenant_id);

-- Subscribers with per-tenant custom fields (JSONB) and consent flags.
CREATE TABLE subscribers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    list_id     UUID REFERENCES lists(id) ON DELETE SET NULL,
    email       TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    fields      JSONB NOT NULL DEFAULT '{}',
    double_opt_in BOOLEAN NOT NULL DEFAULT FALSE,
    unsubscribed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email)
);
CREATE INDEX idx_subscribers_tenant_list ON subscribers(tenant_id, list_id);

-- Suppression list enforced at send time.
CREATE TABLE suppressions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    reason      TEXT NOT NULL, -- bounce | complaint | unsubscribe | manual
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email)
);

-- Campaigns.
CREATE TABLE campaigns (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    list_id     UUID NOT NULL REFERENCES lists(id),
    smtp_account_id UUID REFERENCES smtp_accounts(id),
    subject     TEXT NOT NULL,
    preheader   TEXT NOT NULL DEFAULT '',
    html_body   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'draft', -- draft | queued | sending | paused | completed | failed
    total_count INT NOT NULL DEFAULT 0,
    sent_count  INT NOT NULL DEFAULT 0,
    failed_count INT NOT NULL DEFAULT 0,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_campaigns_tenant_status ON campaigns(tenant_id, status);

-- Per-recipient messages (send state machine).
CREATE TABLE messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscriber_id UUID NOT NULL REFERENCES subscribers(id),
    to_email    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending', -- pending | sent | failed | bounced | complained | unsubscribed
    smtp_msg_id TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    attempts    INT NOT NULL DEFAULT 0,
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (campaign_id, subscriber_id)
);
CREATE INDEX idx_messages_campaign_status ON messages(campaign_id, status);

-- Tracking events: open | click | bounce | complaint | unsubscribe.
CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    campaign_id UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    message_id  UUID REFERENCES messages(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    meta        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_campaign_type ON events(campaign_id, type);
CREATE INDEX idx_events_tenant_time ON events(tenant_id, created_at);

-- Rental plans and subscriptions (volume tiers).
CREATE TABLE plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL UNIQUE,
    price_cents     INT NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'usd',
    email_quota     BIGINT NOT NULL,      -- emails per month
    subscriber_limit BIGINT NOT NULL,
    stripe_price_id TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE tenants ADD CONSTRAINT fk_tenants_plan FOREIGN KEY (plan_id) REFERENCES plans(id);

CREATE TABLE subscriptions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan_id     UUID NOT NULL REFERENCES plans(id),
    status      TEXT NOT NULL DEFAULT 'active',
    period_start TIMESTAMPTZ NOT NULL DEFAULT now(),
    period_end  TIMESTAMPTZ NOT NULL,
    emails_used BIGINT NOT NULL DEFAULT 0,
    stripe_subscription_id TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subscriptions_tenant ON subscriptions(tenant_id);

-- +goose Down
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS plans;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS suppressions;
DROP TABLE IF EXISTS subscribers;
DROP TABLE IF EXISTS lists;
DROP TABLE IF EXISTS sending_domains;
DROP TABLE IF EXISTS smtp_accounts;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
