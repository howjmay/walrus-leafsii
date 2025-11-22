package api

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/leafsii/leafsii-backend/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type Middleware struct {
	logger  *zap.SugaredLogger
	metrics *metrics.Metrics
}

func NewMiddleware(logger *zap.SugaredLogger, metrics *metrics.Metrics) *Middleware {
	return &Middleware{
		logger:  logger,
		metrics: metrics,
	}
}

// CORS middleware
func (m *Middleware) CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	// Allow the configured origins and also mirror the request Origin when it's not present in the list.
	// This keeps dev flows working when accessing the frontend from a LAN IP while the backend listens on localhost.
	return func(next http.Handler) http.Handler {
		base := cors.Handler(cors.Options{
			AllowedOrigins:   allowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: true,
			MaxAge:           300,
		})

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && !originAllowed(origin, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			base(next).ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, o := range allowed {
		if o == origin {
			return true
		}
	}
	return false
}

// Rate limiting middleware
func (m *Middleware) RateLimit(rpm int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(float64(rpm)/60.0), rpm/6) // Allow burst of 1/6th of rpm

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Request logging middleware
func (m *Middleware) RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Special logging for transaction-related endpoints
		isTransactionEndpoint := strings.Contains(r.URL.Path, "/transactions")
		if isTransactionEndpoint {
			m.logger.Infow("Transaction endpoint request detected",
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"content_length", r.ContentLength,
				"headers", getImportantHeaders(r),
			)
		}

		defer func() {
			duration := time.Since(start)

			m.logger.Infow("HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"status", ww.Status(),
				"size", ww.BytesWritten(),
				"duration", duration,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)

			// Special logging for transaction endpoint responses
			if isTransactionEndpoint {
				m.logger.Infow("Transaction endpoint response",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"response_size", ww.BytesWritten(),
					"duration", duration,
				)
			}

			// Record metrics
			m.metrics.RecordHTTPRequest(r.Context(), r.Method, r.URL.Path, ww.Status(), duration)
		}()

		next.ServeHTTP(ww, r)
	})
}

// Helper function to extract important headers for logging
func getImportantHeaders(r *http.Request) map[string]string {
	important := []string{"Content-Type", "Authorization", "X-User-Address", "X-Request-ID", "Origin", "Referer"}
	headers := make(map[string]string)
	
	for _, header := range important {
		if value := r.Header.Get(header); value != "" {
			headers[header] = value
		}
	}
	
	return headers
}

// Security headers middleware
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}

// Compression middleware
func (m *Middleware) Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Check content type (only compress JSON and text)
		contentType := w.Header().Get("Content-Type")
		if !shouldCompress(contentType) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")

		gz := gzip.NewWriter(w)
		defer gz.Close()

		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next.ServeHTTP(gzw, r)
	})
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func shouldCompress(contentType string) bool {
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/") ||
		strings.Contains(contentType, "application/javascript")
}

// Recovery middleware with structured logging
func (m *Middleware) Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				m.logger.Errorw("Panic recovered",
					"panic", rvr,
					"method", r.Method,
					"path", r.URL.Path,
					"remote_addr", r.RemoteAddr,
				)

				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// Request ID middleware
func (m *Middleware) RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := middleware.GetReqID(r.Context())
		if requestID == "" {
			requestID = generateRequestID()
		}

		ctx := context.WithValue(r.Context(), middleware.RequestIDKey, requestID)
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Timeout middleware
func (m *Middleware) Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, "Request timeout")
	}
}

func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
