# go-audit

A reusable audit logging library for Go applications. Provides context propagation, a generic repository interface, HTTP middleware for [chi](https://github.com/go-chi/chi), and a PostgreSQL backend.

## Installation

```bash
go get github.com/kafeiih/go-audit
```

## Features

- **Immutable audit entries** with UUID, timestamps, user info, and arbitrary JSON details
- **Context propagation** — attach and retrieve audit metadata (`Info`) across the request lifecycle
- **Skip mechanism** — mark contexts to bypass audit (e.g. bulk imports)
- **Chi middleware** with a fixed-size worker pool for non-blocking, async persistence
- **PostgreSQL backend** (`pgxaudit`) with session variable injection for database-level triggers
- **Pluggable architecture** — implement `AuditRepository` to use any storage backend

## Quick Start

### 1. Set up the repository and middleware

```go
package main

import (
    "log/slog"
    "net/http"
    "os"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    audit "github.com/kafeiih/go-audit"
    "github.com/kafeiih/go-audit/chiware"
    "github.com/kafeiih/go-audit/pgxaudit"
)

func main() {
    pool, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))

    auditPool := pgxaudit.NewAuditPool(pool)
    repo := pgxaudit.NewPostgresRepo(auditPool)
    logger := slog.Default()

    mw := chiware.NewAuditMiddleware(repo, logger, func(ctx context.Context) *chiware.UserInfo {
        // Extract user from your auth system (Zitadel, Keycloak, etc.)
        return &chiware.UserInfo{
            UserID:   "user-123",
            Username: "oscar",
        }
    })
    defer mw.Shutdown()

    r := chi.NewRouter()
    r.Use(mw.Handler())

    r.Get("/v1/orders/{id}", getOrder)
    r.Post("/v1/orders", createOrder)

    http.ListenAndServe(":8080", r)
}
```

### 2. Propagate audit context in your services

```go
// Attach audit info to context before database calls
ctx = audit.WithInfo(ctx, audit.Info{
    UserID:     "user-123",
    Username:   "oscar",
    Resource:   "orders",
    ResourceID: "ord-456",
    IP:         "192.168.1.1",
    UserAgent:  "MyApp/1.0",
})

// Skip auditing for specific operations
ctx = audit.WithSkipAudit(ctx)
```

### 3. Create audit entries directly

```go
entry, err := audit.NewAuditLog(
    "user-123", "oscar",
    audit.ActionCreate,
    "orders", "ord-456",
    "192.168.1.1", "MyApp/1.0",
    map[string]any{"amount": 100.50},
)
if err != nil {
    log.Fatal(err)
}
repo.Create(ctx, entry)
```

## Architecture

```
audit (core)
├── AuditLog         — immutable log entry
├── Action           — CREATE | READ | UPDATE | DELETE
├── Info             — context-propagated audit metadata
├── AuditRepository  — generic persistence interface
│
├── chiware/
│   └── AuditMiddleware  — chi HTTP middleware with worker pool
│       • 4 workers, 256-entry buffered queue
│       • Maps HTTP methods → audit actions
│       • Extracts resource/ID from chi route patterns
│
└── pgxaudit/
    ├── PostgresRepo     — AuditRepository implementation for PostgreSQL
    └── AuditPool        — pgxpool wrapper that sets session variables
        • Sets app.user_id, app.username, etc. via SET LOCAL
        • Enables database-level audit triggers
```

## Database Schema

```sql
CREATE SCHEMA IF NOT EXISTS audit;

CREATE TABLE audit.audit_logentry (
    id          UUID PRIMARY KEY,
    user_id     TEXT NOT NULL,
    username    TEXT,
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    resource_id TEXT,
    ip          TEXT,
    user_agent  TEXT,
    details     JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL
);

-- Optional queue table for durable retries (outbox pattern)
CREATE TABLE audit.audit_outbox (
    id            BIGSERIAL PRIMARY KEY,
    event_id      UUID NOT NULL,
    payload       JSONB NOT NULL,
    attempts      INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at  TIMESTAMPTZ,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```


## Repository Interface

Implement `AuditRepository` to use a different storage backend:

```go
type AuditRepository interface {
    Create(ctx context.Context, entry *AuditLog) error
    GetByID(ctx context.Context, id uuid.UUID) (*AuditLog, error)
    List(ctx context.Context, filters AuditFilters) ([]AuditLog, int, error)
}
```

`AuditFilters` supports filtering by `UserID`, `Resource`, `Action`, time range (`From`/`To`), and pagination (`Limit`/`Offset`).

## Middleware Behavior

| HTTP Method         | Audit Action |
|---------------------|--------------|
| POST                | CREATE       |
| PUT, PATCH          | UPDATE       |
| DELETE              | DELETE       |
| GET, HEAD, OPTIONS  | READ         |

- Requests to the `audit` resource are automatically skipped
- Unauthenticated requests (nil `UserExtractor` result) are not audited
- When the queue is full, entries are discarded with a warning log

## Testing

```bash
go test ./...
```

## License

MIT
