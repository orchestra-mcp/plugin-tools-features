package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/plugin"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func ListMCPToolsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plugin": map[string]any{"type": "string", "description": "Filter by plugin ID (e.g. tools.features, bridge.claude). Omit for all plugins."},
			"offset": map[string]any{"type": "number", "description": "Skip first N results (default 0)"},
			"limit":  map[string]any{"type": "number", "description": "Max results to return (default 50, max 200)"},
		},
	})
	return s
}

func SearchMCPToolsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query — matches tool names and descriptions"},
			"limit": map[string]any{"type": "number", "description": "Max results to return (default 20)"},
		},
		"required": []any{"query"},
	})
	return s
}

func GetMCPToolSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tool_name": map[string]any{"type": "string", "description": "Exact tool name (e.g. create_feature)"},
		},
		"required": []any{"tool_name"},
	})
	return s
}

// ---------- Handlers ----------

// ListMCPTools returns a paginated list of all registered MCP tools.
func ListMCPTools(catalog plugin.ToolCatalog) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		pluginFilter := helpers.GetString(req.Arguments, "plugin")
		offset := helpers.GetInt(req.Arguments, "offset")
		limit := helpers.GetInt(req.Arguments, "limit")
		if limit <= 0 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		if offset < 0 {
			offset = 0
		}

		entries := catalog.ListCatalog(pluginFilter, offset, limit)
		total := catalog.CatalogCount(pluginFilter)
		plugins := catalog.ListPluginIDs()

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## MCP Tools (%d total", total))
		if pluginFilter != "" {
			sb.WriteString(fmt.Sprintf(", plugin: %s", pluginFilter))
		}
		sb.WriteString(")\n\n")

		// Plugin summary.
		sb.WriteString(fmt.Sprintf("**Plugins:** %s\n\n", strings.Join(plugins, ", ")))

		// Table header.
		sb.WriteString("| # | Tool | Plugin | Category | Description |\n")
		sb.WriteString("|---|------|--------|----------|-------------|\n")

		for i, e := range entries {
			desc := e.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			cat := e.Category
			if len(e.Providers) > 0 {
				cat = fmt.Sprintf("%s (%s)", cat, strings.Join(e.Providers, ","))
			}
			sb.WriteString(fmt.Sprintf("| %d | `%s` | %s | %s | %s |\n",
				offset+i+1, e.Name, e.PluginID, cat, desc))
		}

		sb.WriteString(fmt.Sprintf("\n*Showing %d-%d of %d total*\n",
			offset+1, offset+len(entries), total))

		return helpers.TextResult(sb.String()), nil
	}
}

// SearchMCPTools searches tool names and descriptions.
func SearchMCPTools(catalog plugin.ToolCatalog) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "query"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		query := helpers.GetString(req.Arguments, "query")
		limit := helpers.GetInt(req.Arguments, "limit")
		if limit <= 0 {
			limit = 20
		}

		results := catalog.SearchCatalog(query)
		if len(results) > limit {
			results = results[:limit]
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Search results for %q\n\n", query))

		if len(results) == 0 {
			sb.WriteString("No tools found.\n")
			return helpers.TextResult(sb.String()), nil
		}

		sb.WriteString("| # | Tool | Plugin | Category | Description |\n")
		sb.WriteString("|---|------|--------|----------|-------------|\n")

		for i, e := range results {
			desc := e.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			cat := e.Category
			if len(e.Providers) > 0 {
				cat = fmt.Sprintf("%s (%s)", cat, strings.Join(e.Providers, ","))
			}
			sb.WriteString(fmt.Sprintf("| %d | `%s` | %s | %s | %s |\n",
				i+1, e.Name, e.PluginID, cat, desc))
		}

		sb.WriteString(fmt.Sprintf("\n*Found %d results*\n", len(results)))
		return helpers.TextResult(sb.String()), nil
	}
}

// GetMCPTool returns full details for a single tool including its JSON Schema.
func GetMCPTool(catalog plugin.ToolCatalog) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "tool_name"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		toolName := helpers.GetString(req.Arguments, "tool_name")
		entry := catalog.GetCatalogEntry(toolName)
		if entry == nil {
			return helpers.ErrorResult("not_found",
				fmt.Sprintf("tool %q not found", toolName)), nil
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## %s\n\n", entry.Name))
		sb.WriteString(fmt.Sprintf("- **Plugin:** %s\n", entry.PluginID))
		sb.WriteString(fmt.Sprintf("- **Category:** %s\n", entry.Category))
		if len(entry.Providers) > 0 {
			sb.WriteString(fmt.Sprintf("- **Providers:** %s\n", strings.Join(entry.Providers, ", ")))
		}
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", entry.Description))

		if entry.Schema != "" {
			sb.WriteString(fmt.Sprintf("\n### Input Schema\n\n```json\n%s\n```\n", entry.Schema))
		}

		return helpers.TextResult(sb.String()), nil
	}
}
