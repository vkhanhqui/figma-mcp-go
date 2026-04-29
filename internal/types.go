package internal

// BridgeRequest is sent from the Go server to the Figma plugin over WebSocket.
type BridgeRequest struct {
	Type      string                 `json:"type"`
	RequestID string                 `json:"requestId"`
	NodeIDs   []string               `json:"nodeIds,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
}

// BridgeResponse is received from the Figma plugin over WebSocket.
type BridgeResponse struct {
	Type      string      `json:"type"`
	RequestID string      `json:"requestId"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	// Progress fields — sent mid-operation for long-running commands
	Progress int    `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`
	// Hello fields — sent once by the plugin after ws.onopen so the server
	// can record the plugin version + advertised capabilities. Backwards
	// compatible: missing fields are treated as "legacy plugin".
	PluginVersion string   `json:"pluginVersion,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

// RPCRequest is the wire format for follower → leader /rpc calls.
type RPCRequest struct {
	Tool    string                 `json:"tool"`
	NodeIDs []string               `json:"nodeIds,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// RPCResponse is returned by the leader /rpc endpoint.
type RPCResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// Role represents the current role of this server process.
type Role int

const (
	RoleUnknown  Role = 0
	RoleLeader   Role = 1
	RoleFollower Role = 2
)
