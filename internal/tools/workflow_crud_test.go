package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"google.golang.org/protobuf/types/known/structpb"
)

// setupTestDB redirects globaldb to a temp directory and returns a cleanup func.
func setupTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, ".orchestra", "db"), 0700)
	t.Setenv("HOME", tmpDir)

	// Reset globaldb singleton so it uses the new HOME.
	globaldb.Close()
	t.Cleanup(func() { globaldb.Close() })
}

func wfReq(args map[string]any) *pluginv1.ToolRequest {
	s, _ := structpb.NewStruct(args)
	return &pluginv1.ToolRequest{Arguments: s}
}

func getResultText(resp *pluginv1.ToolResponse) string {
	if resp.Result == nil {
		return ""
	}
	return resp.Result.GetFields()["text"].GetStringValue()
}

// ---------- Create tests ----------

func TestCreateWorkflow_Default(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id": "test-project",
		"name":       "Standard Workflow",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.ErrorMessage)
	}

	text := getResultText(resp)
	if !strings.Contains(text, "WFL-") {
		t.Errorf("expected WFL- ID in output, got: %s", text)
	}
	if !strings.Contains(text, "Standard Workflow") {
		t.Errorf("expected workflow name in output, got: %s", text)
	}
}

func TestCreateWorkflow_MissingRequired(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

func TestCreateWorkflow_DuplicateName(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	// Create first.
	_, _ = handler(context.Background(), wfReq(map[string]any{
		"project_id": "test-project",
		"name":       "My Workflow",
	}))

	// Create duplicate.
	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id": "test-project",
		"name":       "My Workflow",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "already_exists" {
		t.Errorf("expected already_exists, got %q", resp.ErrorCode)
	}
}

func TestCreateWorkflow_CustomStates(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id":    "test-project",
		"name":          "Simple Workflow",
		"initial_state": "open",
		"states": map[string]any{
			"open":   map[string]any{"label": "Open", "terminal": false, "active_work": true},
			"closed": map[string]any{"label": "Closed", "terminal": true, "active_work": false},
		},
		"transitions": []any{
			map[string]any{"from": "open", "to": "closed"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s (code: %s)", resp.ErrorMessage, resp.ErrorCode)
	}

	text := getResultText(resp)
	if !strings.Contains(text, "States:** 2") {
		t.Errorf("expected 2 states, got: %s", text)
	}
}

func TestCreateWorkflow_ValidationError_NoTerminal(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id":    "test-project",
		"name":          "Bad Workflow",
		"initial_state": "open",
		"states": map[string]any{
			"open": map[string]any{"label": "Open", "terminal": false},
		},
		"transitions": []any{},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error for no terminal state, got %q", resp.ErrorCode)
	}
}

// ---------- Get tests ----------

func TestGetWorkflow_ByID(t *testing.T) {
	setupTestDB(t)
	createHandler := CreateWorkflowCRUD(nil)
	getHandler := GetWorkflowCRUD()

	// Create a workflow.
	createResp, _ := createHandler(context.Background(), wfReq(map[string]any{
		"project_id": "test-project",
		"name":       "Test WF",
	}))
	text := getResultText(createResp)
	// Extract WFL-XXX ID.
	id := extractID(text, "WFL-")

	// Get by ID.
	resp, err := getHandler(context.Background(), wfReq(map[string]any{"workflow_id": id}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.ErrorMessage)
	}

	resultText := getResultText(resp)
	if !strings.Contains(resultText, "Test WF") {
		t.Errorf("expected workflow name in detail, got: %s", resultText)
	}
}

func TestGetWorkflow_ByProject(t *testing.T) {
	setupTestDB(t)
	createHandler := CreateWorkflowCRUD(nil)
	getHandler := GetWorkflowCRUD()

	// Create a workflow.
	_, _ = createHandler(context.Background(), wfReq(map[string]any{
		"project_id": "my-project",
		"name":       "Default WF",
	}))

	// Get by project.
	resp, err := getHandler(context.Background(), wfReq(map[string]any{"project_id": "my-project"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.ErrorMessage)
	}

	resultText := getResultText(resp)
	if !strings.Contains(resultText, "Default WF") {
		t.Errorf("expected workflow name, got: %s", resultText)
	}
}

func TestGetWorkflow_NotFound(t *testing.T) {
	setupTestDB(t)
	handler := GetWorkflowCRUD()

	resp, err := handler(context.Background(), wfReq(map[string]any{"workflow_id": "WFL-ZZZ"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "not_found" {
		t.Errorf("expected not_found, got %q", resp.ErrorCode)
	}
}

func TestGetWorkflow_MissingArgs(t *testing.T) {
	setupTestDB(t)
	handler := GetWorkflowCRUD()

	resp, err := handler(context.Background(), wfReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

// ---------- Update tests ----------

func TestUpdateWorkflow(t *testing.T) {
	setupTestDB(t)
	createHandler := CreateWorkflowCRUD(nil)
	updateHandler := UpdateWorkflowCRUD(nil)
	getHandler := GetWorkflowCRUD()

	// Create.
	createResp, _ := createHandler(context.Background(), wfReq(map[string]any{
		"project_id": "test-project",
		"name":       "Original Name",
	}))
	id := extractID(getResultText(createResp), "WFL-")

	// Update name.
	resp, err := updateHandler(context.Background(), wfReq(map[string]any{
		"workflow_id": id,
		"name":       "Updated Name",
		"description": "New description",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.ErrorMessage)
	}

	// Verify.
	getResp, _ := getHandler(context.Background(), wfReq(map[string]any{"workflow_id": id}))
	text := getResultText(getResp)
	if !strings.Contains(text, "Updated Name") {
		t.Errorf("expected updated name, got: %s", text)
	}
	if !strings.Contains(text, "New description") {
		t.Errorf("expected updated description, got: %s", text)
	}
}

func TestUpdateWorkflow_NotFound(t *testing.T) {
	setupTestDB(t)
	handler := UpdateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{"workflow_id": "WFL-ZZZ"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "not_found" {
		t.Errorf("expected not_found, got %q", resp.ErrorCode)
	}
}

// ---------- Delete tests ----------

func TestDeleteWorkflow(t *testing.T) {
	setupTestDB(t)
	createHandler := CreateWorkflowCRUD(nil)
	deleteHandler := DeleteWorkflowCRUD(nil)
	getHandler := GetWorkflowCRUD()

	// Create.
	createResp, _ := createHandler(context.Background(), wfReq(map[string]any{
		"project_id": "test-project",
		"name":       "To Delete",
	}))
	id := extractID(getResultText(createResp), "WFL-")

	// Delete.
	resp, err := deleteHandler(context.Background(), wfReq(map[string]any{"workflow_id": id}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.ErrorMessage)
	}

	// Verify deleted.
	getResp, _ := getHandler(context.Background(), wfReq(map[string]any{"workflow_id": id}))
	if getResp.ErrorCode != "not_found" {
		t.Errorf("expected not_found after delete, got %q", getResp.ErrorCode)
	}
}

func TestDeleteWorkflow_NotFound(t *testing.T) {
	setupTestDB(t)
	handler := DeleteWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{"workflow_id": "WFL-ZZZ"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "not_found" {
		t.Errorf("expected not_found, got %q", resp.ErrorCode)
	}
}

// ---------- List tests ----------

func TestListWorkflows_Empty(t *testing.T) {
	setupTestDB(t)
	handler := ListWorkflowsCRUD()

	resp, err := handler(context.Background(), wfReq(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(resp)
	if !strings.Contains(text, "(0)") {
		t.Errorf("expected 0 workflows, got: %s", text)
	}
}

func TestListWorkflows_Multiple(t *testing.T) {
	setupTestDB(t)
	createHandler := CreateWorkflowCRUD(nil)
	listHandler := ListWorkflowsCRUD()

	// Create 2 workflows in different projects.
	createHandler(context.Background(), wfReq(map[string]any{
		"project_id": "project-a",
		"name":       "WF Alpha",
	}))
	createHandler(context.Background(), wfReq(map[string]any{
		"project_id": "project-b",
		"name":       "WF Beta",
	}))

	// List all.
	resp, _ := listHandler(context.Background(), wfReq(map[string]any{}))
	text := getResultText(resp)
	if !strings.Contains(text, "(2)") {
		t.Errorf("expected 2 workflows, got: %s", text)
	}

	// List filtered by project.
	resp2, _ := listHandler(context.Background(), wfReq(map[string]any{"project_id": "project-a"}))
	text2 := getResultText(resp2)
	if !strings.Contains(text2, "(1)") {
		t.Errorf("expected 1 workflow for project-a, got: %s", text2)
	}
}

// ---------- Validation tests ----------

func TestValidateWorkflowStructure_InvalidInitialState(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id":    "test-project",
		"name":          "Bad Init",
		"initial_state": "nonexistent",
		"states": map[string]any{
			"open":   map[string]any{"label": "Open", "terminal": false},
			"closed": map[string]any{"label": "Closed", "terminal": true},
		},
		"transitions": []any{
			map[string]any{"from": "open", "to": "closed"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.ErrorMessage, "initial_state") {
		t.Errorf("expected error about initial_state, got: %s", resp.ErrorMessage)
	}
}

func TestValidateWorkflowStructure_InvalidTransitionRef(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id":    "test-project",
		"name":          "Bad Trans",
		"initial_state": "open",
		"states": map[string]any{
			"open":   map[string]any{"label": "Open", "terminal": false},
			"closed": map[string]any{"label": "Closed", "terminal": true},
		},
		"transitions": []any{
			map[string]any{"from": "open", "to": "nonexistent"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

func TestValidateWorkflowStructure_InvalidGateRef(t *testing.T) {
	setupTestDB(t)
	handler := CreateWorkflowCRUD(nil)

	resp, err := handler(context.Background(), wfReq(map[string]any{
		"project_id":    "test-project",
		"name":          "Bad Gate Ref",
		"initial_state": "open",
		"states": map[string]any{
			"open":   map[string]any{"label": "Open", "terminal": false},
			"closed": map[string]any{"label": "Closed", "terminal": true},
		},
		"transitions": []any{
			map[string]any{"from": "open", "to": "closed", "gate": "nonexistent_gate"},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

// ---------- Helper ----------

// extractID finds a prefix-XXX ID in the text.
func extractID(text, prefix string) string {
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return ""
	}
	end := idx + len(prefix)
	for end < len(text) && (text[end] == '-' || (text[end] >= 'A' && text[end] <= 'Z')) {
		end++
	}
	return text[idx:end]
}
