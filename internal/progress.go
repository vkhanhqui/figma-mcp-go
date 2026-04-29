package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var progressLogger = log.New(os.Stderr, "[progress] ", 0)

// progressTickInterval is how often we emit a "still working" notification
// when no fresher signal is available from the plugin. Long enough to avoid
// spam, short enough that a client UI showing the spinner doesn't fall back
// to a generic "no response" state.
const progressTickInterval = 5 * time.Second

// startAutoProgress emits a periodic progress notification to the MCP client
// while the surrounding tool handler is executing. It is a heartbeat for
// long-running calls where the plugin itself does not emit progress (e.g.
// figma.exportAsync on a large frame), so the client at least knows the
// server is alive and waiting.
//
// Returns a stop function the caller MUST call when the operation completes
// (defer is the easiest pattern). No-op if the client did not include a
// progressToken in the request, or if the MCP server is not in ctx.
func startAutoProgress(ctx context.Context, req mcp.CallToolRequest, tool string) func() {
	if req.Params.Meta == nil || req.Params.Meta.ProgressToken == nil {
		return func() {}
	}
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return func() {}
	}
	token := req.Params.Meta.ProgressToken

	tickerCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(progressTickInterval)
		defer ticker.Stop()
		elapsed := 0
		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-ticker.C:
				elapsed += int(progressTickInterval.Seconds())
				// progress as elapsed seconds; total left out so the client
				// renders an indeterminate bar rather than a misleading %.
				err := srv.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
					"progressToken": token,
					"progress":      elapsed,
					"message":       fmt.Sprintf("%s still working (%ds)", tool, elapsed),
				})
				if err != nil {
					progressLogger.Printf("send progress for %s: %v", tool, err)
				}
			}
		}
	}()
	return cancel
}
