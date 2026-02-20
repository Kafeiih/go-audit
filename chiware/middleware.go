// Package chiware provides a chi-compatible audit logging middleware
// with a fixed-size worker pool for asynchronous persistence.
package chiware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	audit "github.com/kafeiih/go-audit"
)

const (
	defaultWorkers   = 4
	defaultQueueSize = 256
)

// UserInfo carries the authenticated user identity extracted by the host application.
type UserInfo struct {
	UserID   string
	Username string
}

// UserExtractor is a function that retrieves the current user from the
// request context.  Each host application injects its own implementation
// (e.g. from Zitadel, Keycloak, etc.).
type UserExtractor func(context.Context) *UserInfo

// auditJob holds the captured data needed to write a single audit entry.
type auditJob struct {
	userID        string
	username      string
	correlationID string
	action        audit.Action
	resource      string
	resourceID    string
	ip            string
	userAgent     string
	details       map[string]any
}

// AuditMiddleware records an audit log entry for every authenticated request.
// It uses a fixed-size worker pool with a buffered channel to provide
// backpressure instead of spawning unbounded goroutines.
type AuditMiddleware struct {
	repo      audit.AuditRepository
	logger    *slog.Logger
	extractor UserExtractor
	jobs      chan auditJob
	wg        sync.WaitGroup
}

// NewAuditMiddleware creates an AuditMiddleware backed by repo.
// The extractor function is called on each request to obtain the current user;
// if it returns nil the request is not audited.
func NewAuditMiddleware(repo audit.AuditRepository, logger *slog.Logger, extractor UserExtractor) *AuditMiddleware {
	m := &AuditMiddleware{
		repo:      repo,
		logger:    logger,
		extractor: extractor,
		jobs:      make(chan auditJob, defaultQueueSize),
	}

	m.wg.Add(defaultWorkers)
	for range defaultWorkers {
		go m.worker()
	}

	return m
}

// worker reads jobs from the channel until it is closed.
func (m *AuditMiddleware) worker() {
	defer m.wg.Done()

	for job := range m.jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		entry, err := audit.NewAuditLog(
			job.userID, job.username, job.correlationID,
			job.action,
			job.resource, job.resourceID,
			job.ip, job.userAgent,
			job.details,
		)
		if err != nil {
			m.logger.Error("failed to create audit log entry", "error", err)
			cancel()
			continue
		}
		if err := m.repo.Create(ctx, entry); err != nil {
			m.logger.Error("failed to persist audit log entry",
				"error", err,
				"user_id", job.userID,
				"resource", job.resource,
				"action", job.action,
			)
		}
		cancel()
	}
}

// Shutdown closes the job channel and waits for all workers to finish.
// Call this after http.Server.Shutdown to avoid losing in-flight entries.
func (m *AuditMiddleware) Shutdown() {
	close(m.jobs)
	m.wg.Wait()
}

// Handler returns the chi-compatible middleware function.
func (m *AuditMiddleware) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			user := m.extractor(r.Context())
			if user == nil {
				return
			}

			resource, resourceID := ExtractResource(r)

			// Skip auditing the audit endpoint itself.
			if resource == "audit" {
				return
			}

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			job := auditJob{
				userID:        user.UserID,
				username:      user.Username,
				correlationID: ExtractCorrelationID(r),
				action:        MethodToAction(r.Method),
				resource:      resource,
				resourceID:    resourceID,
				ip:            ExtractIP(r.RemoteAddr),
				userAgent:     r.UserAgent(),
				details: map[string]any{
					"status_code": status,
					"method":      r.Method,
				},
			}

			select {
			case m.jobs <- job:
			default:
				m.logger.Warn("audit log queue full, discarding entry",
					"user_id", job.userID,
					"resource", job.resource,
					"action", job.action,
				)
			}
		})
	}
}

// MethodToAction maps HTTP methods to audit Actions.
func MethodToAction(method string) audit.Action {
	switch method {
	case http.MethodPost:
		return audit.ActionCreate
	case http.MethodPut, http.MethodPatch:
		return audit.ActionUpdate
	case http.MethodDelete:
		return audit.ActionDelete
	default:
		return audit.ActionRead
	}
}

// ExtractResource derives the resource name and resource ID from the request.
// It uses chi's matched route pattern (e.g. /v1/tesoreria/pagos/{id})
// so the value is stable regardless of the actual ID in the URL.
func ExtractResource(r *http.Request) (resource, resourceID string) {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return strings.TrimPrefix(r.URL.Path, "/v1/"), ""
	}

	// Extract last URL param value as resource_id (convention: /{id}).
	params := rctx.URLParams
	if len(params.Values) > 0 {
		resourceID = params.Values[len(params.Values)-1]
	}

	// Build resource from the route pattern, dropping param segments.
	// /v1/tesoreria/pagos/{id} → tesoreria/pagos
	pattern := strings.TrimPrefix(rctx.RoutePattern(), "/v1/")
	parts := strings.Split(pattern, "/")
	clean := parts[:0]
	for _, p := range parts {
		if !strings.HasPrefix(p, "{") && p != "" && p != "*" {
			clean = append(clean, p)
		}
	}
	resource = strings.Join(clean, "/")

	return resource, resourceID
}

// ExtractCorrelationID returns request correlation id from common headers.
func ExtractCorrelationID(r *http.Request) string {
	if v := r.Header.Get("X-Correlation-ID"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Request-ID"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Request-Id"); v != "" {
		return v
	}

	return ""
}

// ExtractIP strips the port from a host:port address.
func ExtractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
