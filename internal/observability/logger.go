// Package observability owns the structured logging and (eventually) Prometheus
// metrics surface for figma-mcp-go. The single global logger is configured from
// environment variables once at startup so the rest of the codebase can pull it
// from anywhere via Logger() without threading it through every constructor.
package observability

import (
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Environment knobs:
//   FIGMA_MCP_LOG          → debug | info | warn | error  (default: info)
//   FIGMA_MCP_LOG_FORMAT   → text | json                 (default: text)
//
// JSON output is the right pick when shipping to a log aggregator (Loki,
// CloudWatch). Text is the friendlier default for terminal use.
const (
	envLogLevel  = "FIGMA_MCP_LOG"
	envLogFormat = "FIGMA_MCP_LOG_FORMAT"
)

var (
	loggerOnce sync.Once
	logger     *slog.Logger
)

// Logger returns the process-wide structured logger. Lazily initialised so
// tests and import-order quirks don't fight us; safe to call concurrently.
func Logger() *slog.Logger {
	loggerOnce.Do(initLogger)
	return logger
}

func initLogger() {
	level := parseLevel(os.Getenv(envLogLevel))
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(os.Getenv(envLogFormat)) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	logger = slog.New(handler)
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Component returns a child logger pre-tagged with `component=name`. Use for
// per-package loggers so structured filters (`component=bridge`) work cleanly.
func Component(name string) *slog.Logger {
	return Logger().With("component", name)
}
