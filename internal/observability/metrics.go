package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metrics surface. The leader HTTP server mounts MetricsHandler()
// at /metrics. All metrics are registered against a private Registry so we
// don't pollute the default global one (which auto-collects Go runtime / process
// metrics — useful, but the consumer should opt in deliberately).
//
// Naming follows the Prometheus convention: `<namespace>_<subsystem>_<name>_<unit>`.
// Namespace `figma_mcp` keeps these separable from any other apps a user might
// scrape from the same node.
var (
	registry = prometheus.NewRegistry()

	// RequestsTotal counts every Send/proxy invocation. Status is "ok",
	// "plugin_error", or "transport_error"; role is "leader" or "follower".
	RequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "figma_mcp",
		Name:      "requests_total",
		Help:      "Total tool calls handled, partitioned by tool, status, and role.",
	}, []string{"tool", "status", "role"})

	// RequestDurationSeconds tracks end-to-end latency from Bridge.Send entry
	// to response. Buckets cover the common range (small reads ~10ms,
	// document export ~10s).
	RequestDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "figma_mcp",
		Name:      "request_duration_seconds",
		Help:      "Latency of tool calls in seconds.",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"tool", "role"})

	// CacheHitsTotal counts (contentHash, imageHash) cache lookups that
	// avoided sending raw bytes over the wire.
	CacheHitsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "figma_mcp",
		Name:      "cache_hits_total",
		Help:      "Image cache lookups that avoided base64 wire payload.",
	}, []string{"tool"})

	// CacheMissesTotal complements CacheHitsTotal so we can compute hit ratio.
	CacheMissesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "figma_mcp",
		Name:      "cache_misses_total",
		Help:      "Image cache lookups that fell through and uploaded bytes.",
	}, []string{"tool"})

	// PendingRequests is the live count of in-flight bridge requests.
	PendingRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "figma_mcp",
		Name:      "pending_requests",
		Help:      "Tool calls currently awaiting a plugin response.",
	})

	// PluginConnected is 1 when the plugin WebSocket is up, 0 otherwise.
	PluginConnected = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "figma_mcp",
		Name:      "plugin_connected",
		Help:      "1 if the Figma plugin is currently connected, else 0.",
	})

	// CacheSize tracks the number of (contentHash → imageHash) entries.
	CacheSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "figma_mcp",
		Name:      "image_cache_size",
		Help:      "Number of entries currently held in the image hash cache.",
	})

	// PluginDisconnectsTotal increments on every observed plugin disconnect.
	// `reason` is "read_error", "write_error", "replaced", or "shutdown".
	PluginDisconnectsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "figma_mcp",
		Name:      "plugin_disconnects_total",
		Help:      "Total observed plugin disconnects, partitioned by reason.",
	}, []string{"reason"})
)

func init() {
	registry.MustRegister(
		RequestsTotal,
		RequestDurationSeconds,
		CacheHitsTotal,
		CacheMissesTotal,
		PendingRequests,
		PluginConnected,
		CacheSize,
		PluginDisconnectsTotal,
	)
}

// MetricsHandler returns an http.Handler suitable for mounting at /metrics.
// It serves the private registry plus Go process collectors registered above.
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		Registry: registry,
	})
}

// Registry exposes the underlying registry for advanced callers (custom
// collectors, tests). Most code should use the package-level metric vars.
func Registry() *prometheus.Registry {
	return registry
}
