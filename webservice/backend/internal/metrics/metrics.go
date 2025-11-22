package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Metrics struct {
	HTTPRequests      metric.Int64Counter
	HTTPDuration      metric.Float64Histogram
	CacheHits         metric.Int64Counter
	CacheMisses       metric.Int64Counter
	ActiveConnections metric.Int64UpDownCounter
}

func Setup(serviceName string) (*Metrics, http.Handler, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	meter := provider.Meter(serviceName)

	m := &Metrics{}

	m.HTTPRequests, err = meter.Int64Counter(
		"fx_http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
	)
	if err != nil {
		return nil, nil, err
	}

	m.HTTPDuration, err = meter.Float64Histogram(
		"fx_http_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
	)
	if err != nil {
		return nil, nil, err
	}

	m.CacheHits, err = meter.Int64Counter(
		"fx_cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
	)
	if err != nil {
		return nil, nil, err
	}

	m.CacheMisses, err = meter.Int64Counter(
		"fx_cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
	)
	if err != nil {
		return nil, nil, err
	}

	m.ActiveConnections, err = meter.Int64UpDownCounter(
		"fx_websocket_connections",
		metric.WithDescription("Number of active WebSocket connections"),
	)
	if err != nil {
		return nil, nil, err
	}

	handler := promhttp.Handler()
	return m, handler, nil
}

func (m *Metrics) RecordHTTPRequest(ctx context.Context, method, path string, status int, duration time.Duration) {
	labels := metric.WithAttributes(
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.Int("status", status),
	)

	m.HTTPRequests.Add(ctx, 1, labels)
	m.HTTPDuration.Record(ctx, duration.Seconds(), labels)
}

func (m *Metrics) RecordCacheHit(ctx context.Context, key string) {
	m.CacheHits.Add(ctx, 1, metric.WithAttributes(attribute.String("key", key)))
}

func (m *Metrics) RecordCacheMiss(ctx context.Context, key string) {
	m.CacheMisses.Add(ctx, 1, metric.WithAttributes(attribute.String("key", key)))
}

func (m *Metrics) IncrementConnections(ctx context.Context) {
	m.ActiveConnections.Add(ctx, 1)
}

func (m *Metrics) DecrementConnections(ctx context.Context) {
	m.ActiveConnections.Add(ctx, -1)
}
