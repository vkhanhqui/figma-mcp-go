package internal

import "github.com/mark3labs/mcp-go/server"

func registerWriteTools(s *server.MCPServer, node *Node) {
	registerWriteCreateTools(s, node)
	registerWriteModifyTools(s, node)
	registerWriteStyleTools(s, node)
	registerWriteVariableTools(s, node)
	registerWriteComponentTools(s, node)
	registerWritePrototypeTools(s, node)
}
