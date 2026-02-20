CREATE SCHEMA IF NOT EXISTS audit;

CREATE TABLE IF NOT EXISTS audit.audit_logentry (
    id             UUID PRIMARY KEY,
    user_id        TEXT NOT NULL DEFAULT '',
    username       TEXT NOT NULL DEFAULT '',
    correlation_id TEXT,
    action         TEXT NOT NULL,
    resource       TEXT NOT NULL DEFAULT '',
    resource_id    TEXT NOT NULL DEFAULT '',
    ip             TEXT NOT NULL DEFAULT '',
    user_agent     TEXT NOT NULL DEFAULT '',
    details        JSONB DEFAULT '{}',

    changed_fields JSONB DEFAULT '{}',

    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit.audit_outbox (
    id            BIGSERIAL PRIMARY KEY,
    event_id      UUID NOT NULL,
    payload       JSONB NOT NULL,
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at  TIMESTAMPTZ,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
