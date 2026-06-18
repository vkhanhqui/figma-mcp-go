package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mark3labs/mcp-go/server"

	"github.com/vkhanhqui/figma-mcp-go/internal"
)

// version is injected at build time:
// go build -ldflags "-X main.version=1.0.0" ./cmd/figma-mcp-go
var version = "dev"

var logger = log.New(os.Stderr, "", 0)

func main() {
	ip := flag.String("ip", "127.0.0.1", "IP address to listen on (use 0.0.0.0 to accept remote connections)")
	port := flag.Int("port", 1994, "port to listen on")
	authToken := flag.String("auth-token", os.Getenv("FIGMA_MCP_AUTH_TOKEN"), "optional shared token required for plugin and follower bridge requests")
	flag.Parse()
	token := strings.TrimSpace(*authToken)

	parsedIP := net.ParseIP(*ip)
	if parsedIP == nil {
		logger.Fatalf("invalid IP address: %q", *ip)
	}
	if token == "" {
		logger.Printf("WARNING: bridge authentication is disabled; set FIGMA_MCP_AUTH_TOKEN or --auth-token to require a shared token")
	} else {
		logger.Printf("Bridge authentication enabled")
	}
	if !parsedIP.IsLoopback() && token == "" {
		logger.Printf("WARNING: binding to %s — server will be reachable from the network with no authentication", *ip)
	} else if !parsedIP.IsLoopback() {
		logger.Printf("WARNING: binding to %s — server will be reachable from the network", *ip)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	node := internal.NewNodeWithAuth(*ip, *port, version, token)
	election := internal.NewElectionWithAuth(*ip, *port, node, token)

	if err := election.Start(ctx); err != nil {
		logger.Fatalf("election start: %v", err)
	}

	logger.Printf("Starting figma-mcp-go %s (role: %s)", version, node.RoleName())

	s := server.NewMCPServer("figma-mcp-go", version)
	internal.RegisterTools(s, node)
	internal.RegisterPrompts(s)

	go func() {
		<-ctx.Done()
		logger.Printf("Shutting down...")
		election.Stop()
		node.Stop()
	}()

	if err := server.ServeStdio(s); err != nil {
		logger.Fatalf("mcp serve: %v", err)
	}
}
