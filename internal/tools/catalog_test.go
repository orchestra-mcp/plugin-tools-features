package tools

import (
	"context"
	"testing"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/plugin"
	"google.golang.org/protobuf/types/known/structpb"
)

// mockCatalog implements plugin.ToolCatalog for testing.
type mockCatalog struct {
	entries []plugin.CatalogEntry
}

func (m *mockCatalog) ListCatalog(pluginFilter string, offset, limit int) []plugin.CatalogEntry {
	var filtered []plugin.CatalogEntry
	for _, e := range m.entries {
		if pluginFilter != "" && e.PluginID != pluginFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	if offset >= len(filtered) {
		return nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end]
}

func (m *mockCatalog) CatalogCount(pluginFilter string) int {
	if pluginFilter == "" {
		return len(m.entries)
	}
	count := 0
	for _, e := range m.entries {
		if e.PluginID == pluginFilter {
			count++
		}
	}
	return count
}

func (m *mockCatalog) SearchCatalog(query string) []plugin.CatalogEntry {
	var results []plugin.CatalogEntry
	for _, e := range m.entries {
		if contains(e.Name, query) || contains(e.Description, query) {
			results = append(results, e)
		}
	}
	return results
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || indexOf(s, sub))
}

func indexOf(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func (m *mockCatalog) GetCatalogEntry(toolName string) *plugin.CatalogEntry {
	for _, e := range m.entries {
		if e.Name == toolName {
			return &e
		}
	}
	return nil
}

func (m *mockCatalog) ListPluginIDs() []string {
	seen := make(map[string]bool)
	var ids []string
	for _, e := range m.entries {
		if !seen[e.PluginID] {
			ids = append(ids, e.PluginID)
			seen[e.PluginID] = true
		}
	}
	return ids
}

func testCatalog() *mockCatalog {
	return &mockCatalog{
		entries: []plugin.CatalogEntry{
			{Name: "create_feature", Description: "Create a new feature", PluginID: "tools.features", Category: "tool"},
			{Name: "list_features", Description: "List all features", PluginID: "tools.features", Category: "tool"},
			{Name: "advance_feature", Description: "Advance a feature to the next workflow status", PluginID: "tools.features", Category: "tool"},
			{Name: "install_pack", Description: "Install a pack from the marketplace", PluginID: "tools.marketplace", Category: "tool"},
			{Name: "ai_prompt", Description: "Send a prompt to an AI model", PluginID: "bridge.claude", Category: "ai", Providers: []string{"claude"}},
		},
	}
}

func makeReq(args map[string]any) *pluginv1.ToolRequest {
	s, _ := structpb.NewStruct(args)
	return &pluginv1.ToolRequest{Arguments: s}
}

func TestListMCPTools_AllTools(t *testing.T) {
	handler := ListMCPTools(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.ErrorMessage)
	}
	text := resp.Result.GetFields()["text"].GetStringValue()
	if text == "" {
		t.Fatal("expected non-empty output")
	}
	// Should contain all 5 tools.
	if !indexOf(text, "5 total") {
		t.Errorf("expected '5 total' in output, got: %s", text[:200])
	}
}

func TestListMCPTools_FilterByPlugin(t *testing.T) {
	handler := ListMCPTools(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{"plugin": "tools.features"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resp.Result.GetFields()["text"].GetStringValue()
	if !indexOf(text, "3 total") {
		t.Errorf("expected '3 total' for tools.features filter, got: %s", text[:200])
	}
}

func TestListMCPTools_Pagination(t *testing.T) {
	handler := ListMCPTools(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{"offset": 2.0, "limit": 2.0}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resp.Result.GetFields()["text"].GetStringValue()
	if !indexOf(text, "Showing 3-4 of 5") {
		t.Errorf("expected pagination info 'Showing 3-4 of 5', got: %s", text)
	}
}

func TestSearchMCPTools_Found(t *testing.T) {
	handler := SearchMCPTools(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{"query": "feature"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resp.Result.GetFields()["text"].GetStringValue()
	if !indexOf(text, "create_feature") {
		t.Error("expected create_feature in results")
	}
}

func TestSearchMCPTools_NoResults(t *testing.T) {
	handler := SearchMCPTools(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{"query": "nonexistent_xyz"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resp.Result.GetFields()["text"].GetStringValue()
	if !indexOf(text, "No tools found") {
		t.Error("expected 'No tools found' message")
	}
}

func TestSearchMCPTools_MissingQuery(t *testing.T) {
	handler := SearchMCPTools(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

func TestGetMCPTool_Found(t *testing.T) {
	handler := GetMCPTool(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{"tool_name": "ai_prompt"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := resp.Result.GetFields()["text"].GetStringValue()
	if !indexOf(text, "ai_prompt") {
		t.Error("expected tool name in output")
	}
	if !indexOf(text, "bridge.claude") {
		t.Error("expected plugin name in output")
	}
	if !indexOf(text, "claude") {
		t.Error("expected provider in output")
	}
}

func TestGetMCPTool_NotFound(t *testing.T) {
	handler := GetMCPTool(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{"tool_name": "nonexistent"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "not_found" {
		t.Errorf("expected not_found error, got %q", resp.ErrorCode)
	}
}

func TestGetMCPTool_MissingToolName(t *testing.T) {
	handler := GetMCPTool(testCatalog())
	resp, err := handler(context.Background(), makeReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}
