package chiware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	audit "github.com/kafeiih/go-audit"
)

// ---------- Mock repository ----------

type mockRepo struct {
	mu      sync.Mutex
	entries []*audit.AuditLog
}

func (m *mockRepo) Create(_ context.Context, entry *audit.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, _ uuid.UUID) (*audit.AuditLog, error) {
	return nil, nil
}

func (m *mockRepo) List(_ context.Context, _ audit.AuditFilters) ([]audit.AuditLog, int, error) {
	return nil, 0, nil
}

func (m *mockRepo) getEntries() []*audit.AuditLog {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*audit.AuditLog, len(m.entries))
	copy(cp, m.entries)
	return cp
}

// ---------- MethodToAction ----------

func TestMethodToAction(t *testing.T) {
	tests := []struct {
		method string
		want   audit.Action
	}{
		{http.MethodPost, audit.ActionCreate},
		{http.MethodPut, audit.ActionUpdate},
		{http.MethodPatch, audit.ActionUpdate},
		{http.MethodDelete, audit.ActionDelete},
		{http.MethodGet, audit.ActionRead},
		{http.MethodHead, audit.ActionRead},
		{http.MethodOptions, audit.ActionRead},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := MethodToAction(tt.method)
			if got != tt.want {
				t.Errorf("MethodToAction(%s) = %s, want %s", tt.method, got, tt.want)
			}
		})
	}
}

// ---------- ExtractIP ----------

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"host:port", "192.168.1.1:8080", "192.168.1.1"},
		{"ipv6 with port", "[::1]:8080", "::1"},
		{"bare IP no port", "192.168.1.1", "192.168.1.1"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractIP(tt.input)
			if got != tt.want {
				t.Errorf("ExtractIP(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- ExtractResource ----------

func TestExtractResource(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		url       string
		wantRes   string
		wantResID string
	}{
		{
			name:      "resource with id",
			pattern:   "/v1/orders/{id}",
			url:       "/v1/orders/abc-123",
			wantRes:   "orders",
			wantResID: "abc-123",
		},
		{
			name:      "nested resource with id",
			pattern:   "/v1/tesoreria/pagos/{id}",
			url:       "/v1/tesoreria/pagos/pay-1",
			wantRes:   "tesoreria/pagos",
			wantResID: "pay-1",
		},
		{
			name:      "collection endpoint",
			pattern:   "/v1/users",
			url:       "/v1/users",
			wantRes:   "users",
			wantResID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()

			var gotRes, gotResID string
			r.HandleFunc(tt.pattern, func(w http.ResponseWriter, r *http.Request) {
				gotRes, gotResID = ExtractResource(r)
			})

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if gotRes != tt.wantRes {
				t.Errorf("resource = %q, want %q", gotRes, tt.wantRes)
			}
			if gotResID != tt.wantResID {
				t.Errorf("resourceID = %q, want %q", gotResID, tt.wantResID)
			}
		})
	}
}

func TestExtractResource_NoRouteContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/orders/abc-123", nil)
	res, resID := ExtractResource(req)

	if res != "orders/abc-123" {
		t.Errorf("resource = %q, want %q", res, "orders/abc-123")
	}
	if resID != "" {
		t.Errorf("resourceID = %q, want empty", resID)
	}
}

// ---------- Middleware Handler ----------

func TestHandler_AuditsAuthenticatedRequest(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.Default()

	mw := NewAuditMiddleware(repo, logger, func(_ context.Context) *UserInfo {
		return &UserInfo{UserID: "u1", Username: "alice"}
	})

	r := chi.NewRouter()
	r.Use(mw.Handler())
	r.Post("/v1/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/orders/ord-1", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("X-Correlation-ID", "corr-123")
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	// Shutdown flushes the worker queue.
	mw.Shutdown()

	entries := repo.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}

	e := entries[0]
	if e.UserID != "u1" {
		t.Errorf("UserID = %q, want %q", e.UserID, "u1")
	}
	if e.Action != audit.ActionCreate {
		t.Errorf("Action = %s, want CREATE", e.Action)
	}
	if e.Resource != "orders" {
		t.Errorf("Resource = %q, want %q", e.Resource, "orders")
	}
	if e.ResourceID != "ord-1" {
		t.Errorf("ResourceID = %q, want %q", e.ResourceID, "ord-1")
	}
	if e.IP != "10.0.0.1" {
		t.Errorf("IP = %q, want %q", e.IP, "10.0.0.1")
	}
	if e.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q, want %q", e.UserAgent, "TestAgent/1.0")
	}
	if e.CorrelationID != "corr-123" {
		t.Errorf("CorrelationID = %q, want %q", e.CorrelationID, "corr-123")
	}
}

func TestHandler_SkipsUnauthenticatedRequest(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.Default()

	mw := NewAuditMiddleware(repo, logger, func(_ context.Context) *UserInfo {
		return nil // unauthenticated
	})

	r := chi.NewRouter()
	r.Use(mw.Handler())
	r.Get("/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	mw.Shutdown()

	if len(repo.getEntries()) != 0 {
		t.Error("expected no audit entries for unauthenticated request")
	}
}

func TestHandler_SkipsAuditResource(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.Default()

	mw := NewAuditMiddleware(repo, logger, func(_ context.Context) *UserInfo {
		return &UserInfo{UserID: "u1", Username: "alice"}
	})

	r := chi.NewRouter()
	r.Use(mw.Handler())
	r.Get("/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/audit", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	mw.Shutdown()

	if len(repo.getEntries()) != 0 {
		t.Error("expected no audit entries for audit resource")
	}
}

func TestShutdown_DrainsQueue(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.Default()

	mw := NewAuditMiddleware(repo, logger, func(_ context.Context) *UserInfo {
		return &UserInfo{UserID: "u1", Username: "alice"}
	})

	r := chi.NewRouter()
	r.Use(mw.Handler())
	r.Get("/v1/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Fire multiple requests.
	for i := range 10 {
		req := httptest.NewRequest(http.MethodGet, "/v1/items/item-"+string(rune('0'+i)), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	mw.Shutdown()

	entries := repo.getEntries()
	if len(entries) != 10 {
		t.Errorf("expected 10 audit entries after shutdown, got %d", len(entries))
	}
}

func TestHandler_RecordsStatusCode(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.Default()

	mw := NewAuditMiddleware(repo, logger, func(_ context.Context) *UserInfo {
		return &UserInfo{UserID: "u1", Username: "alice"}
	})

	r := chi.NewRouter()
	r.Use(mw.Handler())
	r.Delete("/v1/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/orders/ord-99", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	mw.Shutdown()

	entries := repo.getEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	status, ok := entries[0].Details["status_code"]
	if !ok {
		t.Fatal("expected status_code in details")
	}
	if status != 204 {
		t.Errorf("status_code = %v, want 204", status)
	}
}

func TestHandler_QueueFullDiscardsEntry(t *testing.T) {
	repo := &mockRepo{}
	logger := slog.Default()

	// Create middleware and immediately close workers so the queue fills up.
	mw := &AuditMiddleware{
		repo:   repo,
		logger: logger,
		extractor: func(_ context.Context) *UserInfo {
			return &UserInfo{UserID: "u1", Username: "alice"}
		},
		jobs: make(chan auditJob), // unbuffered — always full
	}

	r := chi.NewRouter()
	r.Use(mw.Handler())
	r.Get("/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// This should not block; the entry is discarded.
	done := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
		// OK — request completed without blocking.
	case <-time.After(2 * time.Second):
		t.Fatal("handler blocked on full queue")
	}
}

func TestExtractCorrelationID(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{name: "x-correlation-id", headers: map[string]string{"X-Correlation-ID": "corr-1"}, want: "corr-1"},
		{name: "x-request-id", headers: map[string]string{"X-Request-ID": "req-1"}, want: "req-1"},
		{name: "x-request-id-chi", headers: map[string]string{"X-Request-Id": "req-chi-1"}, want: "req-chi-1"},
		{name: "none", headers: map[string]string{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			if got := ExtractCorrelationID(req); got != tt.want {
				t.Errorf("ExtractCorrelationID() = %q, want %q", got, tt.want)
			}
		})
	}
}
