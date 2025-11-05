package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry holds all Prometheus metrics for LogSieve
type Registry struct {
	// Ingestion metrics
	IngestionLogsTotal    *prometheus.CounterVec
	IngestionBytesTotal   *prometheus.CounterVec
	IngestionDuration     *prometheus.HistogramVec
	IngestionErrorsTotal  *prometheus.CounterVec

	// Deduplication metrics
	DedupLogsTotal          *prometheus.CounterVec
	DedupCacheHitsTotal     *prometheus.CounterVec
	DedupCacheMissesTotal   *prometheus.CounterVec
	DedupRatio              *prometheus.GaugeVec
	DedupPatternsTotal      *prometheus.GaugeVec
	DedupProcessingDuration *prometheus.HistogramVec
	Drain3ClustersTotal     prometheus.Gauge
	Drain3MessagesTotal     prometheus.Counter

	// Output metrics
	OutputLogsTotal       *prometheus.CounterVec
	OutputBytesTotal      *prometheus.CounterVec
	OutputErrorsTotal     *prometheus.CounterVec
	OutputDuration        *prometheus.HistogramVec

	// Buffer metrics
	BufferSize            *prometheus.GaugeVec
	BufferCapacity        *prometheus.GaugeVec
	BufferDroppedTotal    *prometheus.CounterVec

	// Profile metrics
	ProfileHitsTotal      *prometheus.CounterVec
	ProfileMissesTotal    *prometheus.CounterVec

	// System metrics
	BuildInfo             *prometheus.GaugeVec
	StartTimeSeconds      prometheus.Gauge
	UptimeSeconds         prometheus.Gauge

	// HTTP metrics
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	HTTPRequestSize       *prometheus.HistogramVec
	HTTPResponseSize      *prometheus.HistogramVec
}

// NewRegistry creates and registers all Prometheus metrics
func NewRegistry() *Registry {
	r := &Registry{
		// Ingestion metrics
		IngestionLogsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_ingestion_logs_total",
				Help: "Total number of log entries ingested",
			},
			[]string{"source", "profile"},
		),
		IngestionBytesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_ingestion_bytes_total",
				Help: "Total bytes of log data ingested",
			},
			[]string{"source", "profile"},
		),
		IngestionDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "logsieve_ingestion_duration_seconds",
				Help:    "Duration of log ingestion requests",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"source", "profile"},
		),
		IngestionErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_ingestion_errors_total",
				Help: "Total number of ingestion errors",
			},
			[]string{"source", "error_type"},
		),

		// Deduplication metrics
		DedupLogsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_dedup_logs_total",
				Help: "Total number of logs processed by dedup engine",
			},
			[]string{"engine", "action"},
		),
		DedupCacheHitsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_dedup_cache_hits_total",
				Help: "Total number of dedup cache hits",
			},
			[]string{"profile", "cache_type"},
		),
		DedupCacheMissesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_dedup_cache_misses_total",
				Help: "Total number of dedup cache misses",
			},
			[]string{"engine"},
		),
		DedupRatio: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "logsieve_dedup_ratio",
				Help: "Current deduplication ratio (0-1, higher is better)",
			},
			[]string{"profile"},
		),
		DedupPatternsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "logsieve_dedup_patterns_total",
				Help: "Current number of deduplication patterns",
			},
			[]string{"profile"},
		),
		DedupProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "logsieve_dedup_processing_duration_seconds",
				Help:    "Duration of dedup processing operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"profile", "engine"},
		),
		Drain3ClustersTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "logsieve_drain3_clusters_total",
				Help: "Current number of Drain3 log clusters",
			},
		),
		Drain3MessagesTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "logsieve_drain3_messages_total",
				Help: "Total number of messages processed by Drain3",
			},
		),

		// Output metrics
		OutputLogsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_output_logs_total",
				Help: "Total number of logs sent to outputs",
			},
			[]string{"output", "status"},
		),
		OutputBytesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_output_bytes_total",
				Help: "Total bytes sent to outputs",
			},
			[]string{"output"},
		),
		OutputErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_output_errors_total",
				Help: "Total number of output errors",
			},
			[]string{"output", "error_type"},
		),
		OutputDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "logsieve_output_duration_seconds",
				Help:    "Duration of output operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"output"},
		),

		// Buffer metrics
		BufferSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "logsieve_buffer_size",
				Help: "Current number of entries in buffer",
			},
			[]string{"buffer"},
		),
		BufferCapacity: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "logsieve_buffer_capacity",
				Help: "Maximum capacity of buffer",
			},
			[]string{"buffer"},
		),
		BufferDroppedTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_buffer_dropped_total",
				Help: "Total number of entries dropped due to buffer overflow",
			},
			[]string{"buffer", "reason"},
		),

		// Profile metrics
		ProfileHitsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_profile_hits_total",
				Help: "Total number of profile hits",
			},
			[]string{"profile"},
		),
		ProfileMissesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_profile_misses_total",
				Help: "Total number of profile misses",
			},
			[]string{"profile"},
		),

		// System metrics
		BuildInfo: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "logsieve_build_info",
				Help: "Build information (version, commit, build_time)",
			},
			[]string{"version", "commit", "build_time"},
		),
		StartTimeSeconds: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "logsieve_start_time_seconds",
				Help: "Unix timestamp of when LogSieve started",
			},
		),
		UptimeSeconds: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "logsieve_uptime_seconds",
				Help: "Current uptime in seconds",
			},
		),

		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "logsieve_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "logsieve_http_request_duration_seconds",
				Help:    "Duration of HTTP requests",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "logsieve_http_request_size_bytes",
				Help:    "Size of HTTP requests in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "endpoint"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "logsieve_http_response_size_bytes",
				Help:    "Size of HTTP responses in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "endpoint"},
		),
	}

	return r
}

// SetBuildInfo sets the build information metric
func (r *Registry) SetBuildInfo(version, commit, buildTime string) {
	r.BuildInfo.WithLabelValues(version, commit, buildTime).Set(1)
}

// SetStartTime sets the start time metric
func (r *Registry) SetStartTime(timestamp float64) {
	r.StartTimeSeconds.Set(timestamp)
}

// UpdateUptime updates the uptime metric
func (r *Registry) UpdateUptime(seconds float64) {
	r.UptimeSeconds.Set(seconds)
}
