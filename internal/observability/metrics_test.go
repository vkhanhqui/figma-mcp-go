package observability

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsHandler_ExposesNamespacedMetrics(t *testing.T) {
	RequestsTotal.WithLabelValues("get_node", "ok", "leader").Inc()
	CacheHitsTotal.WithLabelValues("import_image").Inc()
	RequestDurationSeconds.WithLabelValues("get_node", "leader").Observe(0.05)
	PendingRequests.Set(0)
	PluginConnected.Set(1)

	srv := httptest.NewServer(MetricsHandler())
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)

	wantSubstrings := []string{
		"figma_mcp_requests_total",
		"figma_mcp_cache_hits_total",
		"figma_mcp_pending_requests",
		"figma_mcp_plugin_connected",
		"figma_mcp_request_duration_seconds",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("metrics body missing %q\nbody=%s", want, out)
		}
	}
}
