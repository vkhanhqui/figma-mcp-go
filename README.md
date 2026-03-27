# figma-mcp-go

Figma MCP Server Without API Rate Limits

A free, open-source Figma MCP server that reads Figma files directly via plugin — no REST API, no rate limits. Works with Cursor, Claude, GitHub Copilot, and any MCP-compatible AI tool.

**Highlights**
- No Figma API token required
- No rate limits — free plan friendly
- Reads live Figma data via plugin bridge
- Works locally — no cloud calls
- Written in Go, distributed via npm

---

## Why this exists

Most Figma MCP servers rely on the **Figma REST API**.

That sounds fine… until you hit this:

| Plan | Limit |
|------|-------|
| Starter / View / Collab | **6 tool calls/month** |
| Pro / Org (Dev seat) | 200 tool calls/day |
| Enterprise | 600 tool calls/day |

If you're experimenting with AI tools, you'll burn through that in minutes.

I didn't have enough money to pay for higher limits.
So I built something that **doesn't use the API at all**.

---

## Installation & Setup

Install via `npx` — no build step required. Watch the setup video or follow the steps below.

[![Watch the video](https://img.youtube.com/vi/DjqyU0GKv9k/sddefault.jpg)](https://youtu.be/DjqyU0GKv9k)

### 1. Configure your AI tool

**.vscode/mcp.json** (Cursor / VS Code / GitHub Copilot)
```json
{
  "servers": {
    "figma-mcp-go": {
      "type": "stdio",
      "command": "npx",
      "args": [
        "-y",
        "@vkhanhqui/figma-mcp-go"
      ]
    }
  }
}
```

**.mcp.json** (Claude and other MCP-compatible tools)
```json
{
  "mcpServers": {
    "figma-mcp-go": {
      "command": "npx",
      "args": ["-y", "@vkhanhqui/figma-mcp-go"]
    }
  }
}
```

### 2. Install the Figma plugin

1. In Figma Desktop: **Plugins → Development → Import plugin from manifest**
2. Select `manifest.json` from the [plugin.zip](https://github.com/vkhanhqui/figma-mcp-go/releases)
3. Run the plugin inside any Figma file

---

## Available Tools

### Document & Selection

| Tool | Description |
|------|-------------|
| `get_document` | Full current page tree |
| `get_metadata` | File name, pages, current page |
| `get_selection` | Currently selected nodes |
| `get_node` | Single node by ID |
| `get_nodes_info` | Multiple nodes by ID |
| `get_design_context` | Depth-limited tree — token-efficient for large files |
| `scan_text_nodes` | All text nodes in a subtree |
| `scan_nodes_by_types` | Nodes matching given type list |

### Styles & Variables

| Tool | Description |
|------|-------------|
| `get_styles` | Paint, text, effect, and grid styles |
| `get_variable_defs` | Variable collections and values |
| `get_local_components` | All components in the file |
| `get_annotations` | Dev-mode annotations |

### Export

| Tool | Description |
|------|-------------|
| `get_screenshot` | Base64 image export of any node |
| `save_screenshots` | Export images to disk (server-side, no API call) |

---

## Related Projects

- [magic-spells/figma-mcp-bridge](https://github.com/magic-spells/figma-mcp-bridge)
- [grab/cursor-talk-to-figma-mcp](https://github.com/grab/cursor-talk-to-figma-mcp)
- [gethopp/figma-mcp-bridge](https://github.com/gethopp/figma-mcp-bridge)

---

## Contributing

Issues and PRs are welcome.
