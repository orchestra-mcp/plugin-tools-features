package internal

import (
	"context"
	"regexp"
	"strings"
	"testing"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/plugin-tools-features/internal/tools"
	"google.golang.org/protobuf/types/known/structpb"
)

// testEnv sets up an in-memory storage backend and returns the feature storage
// wrapper plus a context.
func testEnv() (*storage.FeatureStorage, context.Context) {
	mem := storage.NewInMemoryStorage()
	store := storage.NewFeatureStorage(mem)
	return store, context.Background()
}

// callTool is a test helper that invokes a tool handler with the given
// arguments and returns the response.
func callTool(t *testing.T, handler tools.ToolHandler, args map[string]any) *pluginv1.ToolResponse {
	t.Helper()
	s, err := structpb.NewStruct(args)
	if err != nil {
		t.Fatalf("failed to create args struct: %v", err)
	}
	ctx := context.Background()
	resp, err := handler(ctx, &pluginv1.ToolRequest{
		ToolName:  "test",
		Arguments: s,
	})
	if err != nil {
		t.Fatalf("tool handler returned error: %v", err)
	}
	return resp
}

// resultText extracts the text string from a TextResult ToolResponse.
func resultText(t *testing.T, resp *pluginv1.ToolResponse) string {
	t.Helper()
	if resp.Result == nil {
		t.Fatal("response has no result")
	}
	m := resp.Result.AsMap()
	text, ok := m["text"].(string)
	if !ok {
		t.Fatal("response result has no 'text' field")
	}
	return text
}

// featureIDPattern matches FEAT-XXXXXXXX IDs in text.
var featureIDPattern = regexp.MustCompile(`FEAT-[A-Z0-9]+`)

// createTestProject creates a project and returns the store.
func createTestProject(t *testing.T, store *storage.FeatureStorage, name string) {
	t.Helper()
	resp := callTool(t, tools.CreateProject(store), map[string]any{
		"name": name,
	})
	if !resp.Success {
		t.Fatalf("create_project failed: %s", resp.ErrorMessage)
	}
}

// createTestFeature creates a feature and returns the feature ID.
func createTestFeature(t *testing.T, store *storage.FeatureStorage, projectID, title string) string {
	t.Helper()
	resp := callTool(t, tools.CreateFeature(store), map[string]any{
		"project_id": projectID,
		"title":      title,
	})
	if !resp.Success {
		t.Fatalf("create_feature failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	id := featureIDPattern.FindString(text)
	if id == "" {
		t.Fatal("create_feature did not return a feature ID in the text")
	}
	return id
}

// createTestFeatureWithKind creates a feature with a specific kind and returns the feature ID.
func createTestFeatureWithKind(t *testing.T, store *storage.FeatureStorage, projectID, title, kind string) string {
	t.Helper()
	resp := callTool(t, tools.CreateFeature(store), map[string]any{
		"project_id": projectID,
		"title":      title,
		"kind":       kind,
	})
	if !resp.Success {
		t.Fatalf("create_feature (kind=%s) failed: %s", kind, resp.ErrorMessage)
	}
	text := resultText(t, resp)
	id := featureIDPattern.FindString(text)
	if id == "" {
		t.Fatal("create_feature did not return a feature ID in the text")
	}
	return id
}

// Gate evidence constants for the new simplified workflow.
const (
	// Gate: in-progress -> in-testing (Code Complete gate, requires ## Changes with file paths).
	codeCompleteEvidence = "## Changes\n- src/handler.go (added new endpoint for authentication)\n- src/service.go (business logic for token validation)\n- src/models.go (new user model fields)"

	// Gate: in-testing -> in-docs (Test Complete gate, requires ## Results with test file paths).
	testCompleteEvidence = "## Results\n- src/handler_test.go (12 test cases, all passing)\n- src/service_test.go (8 test cases covering edge cases)\n\nAll tests pass with 87% coverage."

	// Gate: in-docs -> in-review (Docs Complete gate, requires ## Docs with .md files in docs/ folder).
	docsCompleteEvidence = "## Docs\n- docs/api/authentication.md (full API reference)\n- docs/guides/setup.md (setup instructions updated)"
)

// advanceWithGateEvidence is a test helper that advances a feature through a
// transition, providing the correct gate evidence for gated transitions.
func advanceWithGateEvidence(t *testing.T, store *storage.FeatureStorage, projectID, featureID, from string) {
	t.Helper()
	args := map[string]any{
		"project_id": projectID,
		"feature_id": featureID,
	}
	switch from {
	case "in-progress":
		args["evidence"] = codeCompleteEvidence
	case "in-testing":
		args["evidence"] = testCompleteEvidence
	case "in-docs":
		args["evidence"] = docsCompleteEvidence
	default:
		// All transitions via advance_feature require evidence now.
		args["evidence"] = "## Changes\n- placeholder/file.go (transition evidence)"
	}
	resp := callTool(t, tools.AdvanceFeature(store), args)
	if !resp.Success {
		t.Fatalf("advance from %s failed: %s", from, resp.ErrorMessage)
	}
}

// startFeature uses set_current_feature to move a feature from todo to in-progress.
func startFeature(t *testing.T, store *storage.FeatureStorage, projectID, featureID string) {
	t.Helper()
	resp := callTool(t, tools.SetCurrentFeature(store), map[string]any{
		"project_id": projectID,
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("set_current_feature failed: %s", resp.ErrorMessage)
	}
}

func TestCreateAndGetProject(t *testing.T) {
	store, _ := testEnv()

	// Create project.
	resp := callTool(t, tools.CreateProject(store), map[string]any{
		"name":        "My App",
		"description": "A test project",
	})
	if !resp.Success {
		t.Fatalf("create_project failed: %s", resp.ErrorMessage)
	}

	// Get project status.
	resp = callTool(t, tools.GetProjectStatus(store), map[string]any{
		"project_id": "my-app",
	})
	if !resp.Success {
		t.Fatalf("get_project_status failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "My App") {
		t.Errorf("expected project name 'My App' in result, got:\n%s", text)
	}
	if !strings.Contains(text, "my-app") {
		t.Errorf("expected project slug 'my-app' in result, got:\n%s", text)
	}

	// List projects.
	resp = callTool(t, tools.ListProjects(store), map[string]any{})
	if !resp.Success {
		t.Fatalf("list_projects failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "My App") {
		t.Errorf("expected 'My App' in project list, got:\n%s", text)
	}
	if !strings.Contains(text, "1") {
		t.Errorf("expected count in project list, got:\n%s", text)
	}
}

func TestCreateAndGetFeature(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Test Project")

	// Create feature.
	resp := callTool(t, tools.CreateFeature(store), map[string]any{
		"project_id":  "test-project",
		"title":       "User Authentication",
		"description": "Implement OAuth2 login flow",
		"priority":    "P0",
	})
	if !resp.Success {
		t.Fatalf("create_feature failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	featureID := featureIDPattern.FindString(text)
	if featureID == "" {
		t.Fatal("create_feature did not return a feature ID")
	}

	// Features now start in "todo" status (not "backlog").
	if !strings.Contains(text, "todo") {
		t.Errorf("expected status todo in result, got:\n%s", text)
	}
	if !strings.Contains(text, "P0") {
		t.Errorf("expected priority P0 in result, got:\n%s", text)
	}

	// Get feature.
	resp = callTool(t, tools.GetFeature(store), map[string]any{
		"project_id": "test-project",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_feature failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "User Authentication") {
		t.Errorf("expected title in get_feature result, got:\n%s", text)
	}
	// Should contain the body content.
	if !strings.Contains(text, "Implement OAuth2 login flow") {
		t.Errorf("expected body content in get_feature result, got:\n%s", text)
	}
}

func TestListFeatures(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "List Test")

	createTestFeature(t, store, "list-test", "Feature A")
	createTestFeature(t, store, "list-test", "Feature B")
	createTestFeature(t, store, "list-test", "Feature C")

	resp := callTool(t, tools.ListFeatures(store), map[string]any{
		"project_id": "list-test",
	})
	if !resp.Success {
		t.Fatalf("list_features failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "Feature A") {
		t.Errorf("expected 'Feature A' in list, got:\n%s", text)
	}
	if !strings.Contains(text, "Feature B") {
		t.Errorf("expected 'Feature B' in list, got:\n%s", text)
	}
	if !strings.Contains(text, "Feature C") {
		t.Errorf("expected 'Feature C' in list, got:\n%s", text)
	}
	if !strings.Contains(text, "3") {
		t.Errorf("expected count '3' in header, got:\n%s", text)
	}

	// Test status filter.
	resp = callTool(t, tools.ListFeatures(store), map[string]any{
		"project_id": "list-test",
		"status":     "in-progress",
	})
	if !resp.Success {
		t.Fatalf("list_features with status filter failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "No features found") {
		t.Errorf("expected 'No features found' for in-progress filter, got:\n%s", text)
	}
}

func TestWorkflowAdvance(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Workflow Test")
	featureID := createTestFeature(t, store, "workflow-test", "Auth Feature")

	// Feature starts as todo. Use set_current_feature to move to in-progress.
	startFeature(t, store, "workflow-test", featureID)

	// in-progress -> in-testing (Code Complete gate: ## Changes with file paths).
	advanceWithGateEvidence(t, store, "workflow-test", featureID, "in-progress")

	// in-testing -> in-docs (Test Complete gate: ## Results with test file paths).
	advanceWithGateEvidence(t, store, "workflow-test", featureID, "in-testing")

	// in-docs -> in-review (Docs Complete gate: ## Docs with .md files in docs/).
	advanceWithGateEvidence(t, store, "workflow-test", featureID, "in-docs")

	// in-review -> done uses submit_review (not advance_feature).
	resp := callTool(t, tools.SubmitReview(store), map[string]any{
		"project_id": "workflow-test",
		"feature_id": featureID,
		"status":     "approved",
		"comment":    "Looks good, approved.",
	})
	if !resp.Success {
		t.Fatalf("submit_review failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "done") {
		t.Errorf("expected 'done' in submit_review result, got:\n%s", text)
	}

	// Advancing from done should fail (evidence is required but status is terminal).
	resp = callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "workflow-test",
		"feature_id": featureID,
		"evidence":   "## Changes\n- some/file.go (trying to advance from done)",
	})
	if resp.Success {
		t.Error("expected advance from done to fail")
	}
	if resp.ErrorCode != "workflow_error" {
		t.Errorf("expected workflow_error, got %q", resp.ErrorCode)
	}
}

func TestWorkflowReject(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Reject Test")
	featureID := createTestFeature(t, store, "reject-test", "Rejectable Feature")

	// Move through full workflow to in-review.
	startFeature(t, store, "reject-test", featureID)
	advanceWithGateEvidence(t, store, "reject-test", featureID, "in-progress")
	advanceWithGateEvidence(t, store, "reject-test", featureID, "in-testing")
	advanceWithGateEvidence(t, store, "reject-test", featureID, "in-docs")

	// Reject from in-review.
	resp := callTool(t, tools.RejectFeature(store), map[string]any{
		"project_id": "reject-test",
		"feature_id": featureID,
		"reason":     "Missing error handling",
	})
	if !resp.Success {
		t.Fatalf("reject_feature failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-review") {
		t.Errorf("expected 'in-review' in rejection text, got:\n%s", text)
	}
	if !strings.Contains(text, "needs-edits") {
		t.Errorf("expected 'needs-edits' in rejection text, got:\n%s", text)
	}

	// From needs-edits, can go back to in-progress via set_current_feature.
	resp = callTool(t, tools.SetCurrentFeature(store), map[string]any{
		"project_id": "reject-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("set_current_feature from needs-edits failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "in-progress") {
		t.Errorf("expected 'in-progress' in set_current result, got:\n%s", text)
	}
}

func TestDependencies(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Deps Test")
	feat1 := createTestFeature(t, store, "deps-test", "Foundation")
	feat2 := createTestFeature(t, store, "deps-test", "Build on Foundation")

	// Add dependency: feat2 depends on feat1.
	resp := callTool(t, tools.AddDependency(store), map[string]any{
		"project_id":    "deps-test",
		"feature_id":    feat2,
		"depends_on_id": feat1,
	})
	if !resp.Success {
		t.Fatalf("add_dependency failed: %s", resp.ErrorMessage)
	}

	// Get dependency graph.
	resp = callTool(t, tools.GetDependencyGraph(store), map[string]any{
		"project_id": "deps-test",
	})
	if !resp.Success {
		t.Fatalf("get_dependency_graph failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, feat2) {
		t.Errorf("expected feat2 %s in dependency graph, got:\n%s", feat2, text)
	}
	if !strings.Contains(text, feat1) {
		t.Errorf("expected feat1 %s in dependency graph, got:\n%s", feat1, text)
	}
	if !strings.Contains(text, "depends on") {
		t.Errorf("expected 'depends on' in dependency graph, got:\n%s", text)
	}

	// Verify feat2 shows as blocked.
	resp = callTool(t, tools.GetBlockedFeatures(store), map[string]any{
		"project_id": "deps-test",
	})
	if !resp.Success {
		t.Fatalf("get_blocked_features failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, feat2) {
		t.Errorf("expected blocked feature %s in result, got:\n%s", feat2, text)
	}
	if !strings.Contains(text, "1") {
		t.Errorf("expected count 1 in blocked features header, got:\n%s", text)
	}

	// Remove dependency.
	resp = callTool(t, tools.RemoveDependency(store), map[string]any{
		"project_id":    "deps-test",
		"feature_id":    feat2,
		"depends_on_id": feat1,
	})
	if !resp.Success {
		t.Fatalf("remove_dependency failed: %s", resp.ErrorMessage)
	}

	// Verify no edges remain.
	resp = callTool(t, tools.GetDependencyGraph(store), map[string]any{
		"project_id": "deps-test",
	})
	if !resp.Success {
		t.Fatalf("get_dependency_graph after remove failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "No dependencies found") {
		t.Errorf("expected 'No dependencies found' after removal, got:\n%s", text)
	}
}

func TestLabels(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Labels Test")
	featureID := createTestFeature(t, store, "labels-test", "Labeled Feature")

	// Add labels.
	resp := callTool(t, tools.AddLabels(store), map[string]any{
		"project_id": "labels-test",
		"feature_id": featureID,
		"labels":     []any{"backend", "urgent"},
	})
	if !resp.Success {
		t.Fatalf("add_labels failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "backend") {
		t.Errorf("expected 'backend' in labels result, got:\n%s", text)
	}
	if !strings.Contains(text, "urgent") {
		t.Errorf("expected 'urgent' in labels result, got:\n%s", text)
	}

	// Add duplicate label (should not duplicate).
	resp = callTool(t, tools.AddLabels(store), map[string]any{
		"project_id": "labels-test",
		"feature_id": featureID,
		"labels":     []any{"backend", "frontend"},
	})
	if !resp.Success {
		t.Fatalf("add_labels (with duplicate) failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "frontend") {
		t.Errorf("expected 'frontend' in labels result, got:\n%s", text)
	}

	// Remove a label.
	resp = callTool(t, tools.RemoveLabels(store), map[string]any{
		"project_id": "labels-test",
		"feature_id": featureID,
		"labels":     []any{"urgent"},
	})
	if !resp.Success {
		t.Fatalf("remove_labels failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if strings.Contains(text, "urgent") {
		t.Errorf("expected 'urgent' removed from labels, got:\n%s", text)
	}
	if !strings.Contains(text, "backend") {
		t.Errorf("expected 'backend' still in labels, got:\n%s", text)
	}
}

func TestAssignment(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Assign Test")
	featureID := createTestFeature(t, store, "assign-test", "Assignable Feature")

	// Assign feature.
	resp := callTool(t, tools.AssignFeature(store), map[string]any{
		"project_id": "assign-test",
		"feature_id": featureID,
		"assignee":   "alice",
	})
	if !resp.Success {
		t.Fatalf("assign_feature failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "alice") {
		t.Errorf("expected 'alice' in assign result, got:\n%s", text)
	}

	// Verify via get_feature.
	resp = callTool(t, tools.GetFeature(store), map[string]any{
		"project_id": "assign-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_feature failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "alice") {
		t.Errorf("expected assignee 'alice' in get_feature result, got:\n%s", text)
	}

	// Unassign feature.
	resp = callTool(t, tools.UnassignFeature(store), map[string]any{
		"project_id": "assign-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("unassign_feature failed: %s", resp.ErrorMessage)
	}

	// Verify unassigned via get_feature -- assignee should not appear.
	resp = callTool(t, tools.GetFeature(store), map[string]any{
		"project_id": "assign-test",
		"feature_id": featureID,
	})
	text = resultText(t, resp)
	if strings.Contains(text, "Assignee") {
		t.Errorf("expected no assignee field after unassign, got:\n%s", text)
	}
}

func TestSearch(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Search Test")

	createTestFeature(t, store, "search-test", "User Authentication")
	createTestFeature(t, store, "search-test", "Database Migration")
	createTestFeature(t, store, "search-test", "User Profile Page")

	// Search for "user".
	resp := callTool(t, tools.SearchFeatures(store), map[string]any{
		"project_id": "search-test",
		"query":      "user",
	})
	if !resp.Success {
		t.Fatalf("search_features failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "User Authentication") {
		t.Errorf("expected 'User Authentication' in search results, got:\n%s", text)
	}
	if !strings.Contains(text, "User Profile Page") {
		t.Errorf("expected 'User Profile Page' in search results, got:\n%s", text)
	}
	if !strings.Contains(text, "2") {
		t.Errorf("expected count '2' in search results header, got:\n%s", text)
	}

	// Search for something that does not exist.
	resp = callTool(t, tools.SearchFeatures(store), map[string]any{
		"project_id": "search-test",
		"query":      "nonexistent",
	})
	if !resp.Success {
		t.Fatalf("search_features (no results) failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "No features found") {
		t.Errorf("expected 'No features found' for empty search, got:\n%s", text)
	}
}

func TestEstimate(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Estimate Test")
	featureID := createTestFeature(t, store, "estimate-test", "Estimable Feature")

	resp := callTool(t, tools.SetEstimate(store), map[string]any{
		"project_id": "estimate-test",
		"feature_id": featureID,
		"estimate":   "L",
	})
	if !resp.Success {
		t.Fatalf("set_estimate failed: %s", resp.ErrorMessage)
	}

	// Verify via get_feature.
	resp = callTool(t, tools.GetFeature(store), map[string]any{
		"project_id": "estimate-test",
		"feature_id": featureID,
	})
	text := resultText(t, resp)
	if !strings.Contains(text, "L") {
		t.Errorf("expected estimate 'L' in get_feature result, got:\n%s", text)
	}

	// Invalid estimate.
	resp = callTool(t, tools.SetEstimate(store), map[string]any{
		"project_id": "estimate-test",
		"feature_id": featureID,
		"estimate":   "XXL",
	})
	if resp.Success {
		t.Error("expected set_estimate with invalid value to fail")
	}
}

func TestNotes(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Notes Test")
	featureID := createTestFeature(t, store, "notes-test", "Notable Feature")

	resp := callTool(t, tools.SaveNote(store), map[string]any{
		"project_id": "notes-test",
		"feature_id": featureID,
		"note":       "This is an important note.",
	})
	if !resp.Success {
		t.Fatalf("save_note failed: %s", resp.ErrorMessage)
	}

	resp = callTool(t, tools.ListNotes(store), map[string]any{
		"project_id": "notes-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("list_notes failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "This is an important note.") {
		t.Errorf("expected note content in list_notes result, got:\n%s", text)
	}
}

func TestUpdateFeature(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Update Test")
	featureID := createTestFeature(t, store, "update-test", "Original Title")

	resp := callTool(t, tools.UpdateFeature(store), map[string]any{
		"project_id": "update-test",
		"feature_id": featureID,
		"title":      "Updated Title",
		"priority":   "P1",
	})
	if !resp.Success {
		t.Fatalf("update_feature failed: %s", resp.ErrorMessage)
	}

	resp = callTool(t, tools.GetFeature(store), map[string]any{
		"project_id": "update-test",
		"feature_id": featureID,
	})
	text := resultText(t, resp)
	if !strings.Contains(text, "Updated Title") {
		t.Errorf("expected 'Updated Title' in get_feature result, got:\n%s", text)
	}
	if !strings.Contains(text, "P1") {
		t.Errorf("expected 'P1' in get_feature result, got:\n%s", text)
	}
}

func TestDeleteProject(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Delete Me")
	createTestFeature(t, store, "delete-me", "Feature in deleted project")

	resp := callTool(t, tools.DeleteProject(store), map[string]any{
		"project_id": "delete-me",
	})
	if !resp.Success {
		t.Fatalf("delete_project failed: %s", resp.ErrorMessage)
	}

	// Verify project is gone.
	resp = callTool(t, tools.GetProjectStatus(store), map[string]any{
		"project_id": "delete-me",
	})
	if resp.Success {
		t.Error("expected get_project_status to fail after deletion")
	}
}

func TestDeleteFeature(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Del Feature Test")
	featureID := createTestFeature(t, store, "del-feature-test", "To Delete")

	resp := callTool(t, tools.DeleteFeature(store), map[string]any{
		"project_id": "del-feature-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("delete_feature failed: %s", resp.ErrorMessage)
	}

	// Verify feature is gone.
	resp = callTool(t, tools.GetFeature(store), map[string]any{
		"project_id": "del-feature-test",
		"feature_id": featureID,
	})
	if resp.Success {
		t.Error("expected get_feature to fail after deletion")
	}
}

func TestGetNextFeature(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Next Test")
	_ = createTestFeature(t, store, "next-test", "Low Priority Feature")
	id2 := createTestFeature(t, store, "next-test", "High Priority Feature")

	// Set one feature to P0.
	callTool(t, tools.UpdateFeature(store), map[string]any{
		"project_id": "next-test",
		"feature_id": id2,
		"priority":   "P0",
	})

	// Features already start as todo, so get_next_feature (which defaults to
	// filtering by todo status) should find them immediately.
	resp := callTool(t, tools.GetNextFeature(store), map[string]any{
		"project_id": "next-test",
	})
	if !resp.Success {
		t.Fatalf("get_next_feature failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, id2) {
		t.Errorf("expected P0 feature %s in next feature result, got:\n%s", id2, text)
	}
	if !strings.Contains(text, "High Priority Feature") {
		t.Errorf("expected 'High Priority Feature' in next feature result, got:\n%s", text)
	}
}

func TestReviewWorkflow(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Review Test")
	featureID := createTestFeature(t, store, "review-test", "Reviewable Feature")

	// Move through full workflow to in-review.
	startFeature(t, store, "review-test", featureID)
	advanceWithGateEvidence(t, store, "review-test", featureID, "in-progress")
	advanceWithGateEvidence(t, store, "review-test", featureID, "in-testing")
	advanceWithGateEvidence(t, store, "review-test", featureID, "in-docs")

	// Check pending reviews -- feature should be in-review now.
	resp := callTool(t, tools.GetPendingReviews(store), map[string]any{
		"project_id": "review-test",
	})
	if !resp.Success {
		t.Fatalf("get_pending_reviews failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, featureID) {
		t.Errorf("expected feature %s in pending reviews, got:\n%s", featureID, text)
	}

	// Submit review as needs-edits.
	resp = callTool(t, tools.SubmitReview(store), map[string]any{
		"project_id": "review-test",
		"feature_id": featureID,
		"status":     "needs-edits",
		"comment":    "Needs more tests",
	})
	if !resp.Success {
		t.Fatalf("submit_review failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "needs-edits") {
		t.Errorf("expected 'needs-edits' in review result, got:\n%s", text)
	}
}

func TestProgress(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Progress Test")
	createTestFeature(t, store, "progress-test", "Feature 1")
	createTestFeature(t, store, "progress-test", "Feature 2")

	resp := callTool(t, tools.GetProgress(store), map[string]any{
		"project_id": "progress-test",
	})
	if !resp.Success {
		t.Fatalf("get_progress failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "2") {
		t.Errorf("expected total '2' in progress, got:\n%s", text)
	}
	if !strings.Contains(text, "0.0%") {
		t.Errorf("expected '0.0%%' completion in progress, got:\n%s", text)
	}
}

func TestWIPLimits(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "WIP Test")

	// Set WIP limit.
	resp := callTool(t, tools.SetWIPLimits(store), map[string]any{
		"project_id":      "wip-test",
		"max_in_progress": 2.0,
	})
	if !resp.Success {
		t.Fatalf("set_wip_limits failed: %s", resp.ErrorMessage)
	}

	// Get WIP limits.
	resp = callTool(t, tools.GetWIPLimits(store), map[string]any{
		"project_id": "wip-test",
	})
	if !resp.Success {
		t.Fatalf("get_wip_limits failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "2") {
		t.Errorf("expected WIP limit '2' in result, got:\n%s", text)
	}

	// Check WIP limit (should be within since nothing is in-progress).
	resp = callTool(t, tools.CheckWIPLimit(store), map[string]any{
		"project_id": "wip-test",
	})
	if !resp.Success {
		t.Fatalf("check_wip_limit failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "within limit") {
		t.Errorf("expected 'within limit' with 0 in-progress, got:\n%s", text)
	}
}

// TestValidation verifies that tools reject missing required arguments.
func TestValidation(t *testing.T) {
	store, _ := testEnv()

	tests := []struct {
		name    string
		handler tools.ToolHandler
		args    map[string]any
	}{
		{"create_project missing name", tools.CreateProject(store), map[string]any{}},
		{"create_feature missing project_id", tools.CreateFeature(store), map[string]any{"title": "x"}},
		{"create_feature missing title", tools.CreateFeature(store), map[string]any{"project_id": "x"}},
		{"get_feature missing args", tools.GetFeature(store), map[string]any{}},
		{"assign missing assignee", tools.AssignFeature(store), map[string]any{"project_id": "x", "feature_id": "y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := callTool(t, tt.handler, tt.args)
			if resp.Success {
				t.Error("expected validation failure")
			}
			if resp.ErrorCode != "validation_error" {
				t.Errorf("expected validation_error, got %q", resp.ErrorCode)
			}
		})
	}
}

// ---------- Gate enforcement tests ----------

func TestGateBlocksWithoutEvidence(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Gate Block Test")
	featureID := createTestFeature(t, store, "gate-block-test", "Gated Feature")

	// Move to in-progress via set_current_feature.
	startFeature(t, store, "gate-block-test", featureID)

	// advance_feature now requires evidence as a required parameter.
	// Calling without evidence should fail with validation_error.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "gate-block-test",
		"feature_id": featureID,
	})
	if resp.Success {
		t.Fatal("expected advance_feature to fail without evidence")
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

func TestGateBlocksWithMissingSections(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Gate Missing Test")
	featureID := createTestFeature(t, store, "gate-missing-test", "Missing Sections Feature")

	// Move to in-progress via set_current_feature.
	startFeature(t, store, "gate-missing-test", featureID)

	// Provide evidence with wrong section (## Summary instead of ## Changes).
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "gate-missing-test",
		"feature_id": featureID,
		"evidence":   "## Summary\nImplemented the entire login flow with full OAuth2 support including token refresh, PKCE verification, and error handling for all edge cases.",
	})
	if resp.Success {
		t.Fatal("expected gate to block with missing ## Changes section")
	}
	if resp.ErrorCode != "gate_blocked" {
		t.Errorf("expected gate_blocked, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.ErrorMessage, "Changes") {
		t.Errorf("expected error to mention 'Changes' section, got:\n%s", resp.ErrorMessage)
	}
}

func TestGateBlocksWithEmptySections(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Gate Empty Test")
	featureID := createTestFeature(t, store, "gate-empty-test", "Empty Sections Feature")

	// Move to in-progress via set_current_feature.
	startFeature(t, store, "gate-empty-test", featureID)

	// ## Changes section is present but empty (no file paths).
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "gate-empty-test",
		"feature_id": featureID,
		"evidence":   "## Changes\n\n\n",
	})
	if resp.Success {
		t.Fatal("expected gate to block with empty section content")
	}
	if resp.ErrorCode != "gate_blocked" {
		t.Errorf("expected gate_blocked, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.ErrorMessage, "insufficient content") {
		t.Errorf("expected 'insufficient content' in error, got:\n%s", resp.ErrorMessage)
	}
}

func TestGatePassesWithValidEvidence(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Gate Pass Test")
	featureID := createTestFeature(t, store, "gate-pass-test", "Valid Evidence Feature")

	// Move to in-progress via set_current_feature.
	startFeature(t, store, "gate-pass-test", featureID)

	// Code Complete gate: valid evidence with ## Changes and file paths.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "gate-pass-test",
		"feature_id": featureID,
		"evidence":   codeCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected gate to pass with valid evidence, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-testing") {
		t.Errorf("expected in-testing in result, got:\n%s", text)
	}
}

func TestGate2PassesWithValidEvidence(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Gate2 Pass Test")
	featureID := createTestFeature(t, store, "gate2-pass-test", "Gate2 Feature")

	// Move through: todo -> in-progress -> in-testing.
	startFeature(t, store, "gate2-pass-test", featureID)
	advanceWithGateEvidence(t, store, "gate2-pass-test", featureID, "in-progress")

	// Test Complete gate: valid evidence with ## Results and test file paths.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "gate2-pass-test",
		"feature_id": featureID,
		"evidence":   testCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected gate 2 to pass, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-docs") {
		t.Errorf("expected in-docs in result, got:\n%s", text)
	}
}

func TestSetCurrentFeatureFromTodo(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Start Test")
	featureID := createTestFeature(t, store, "start-test", "Startable Feature")

	// set_current_feature moves todo -> in-progress.
	resp := callTool(t, tools.SetCurrentFeature(store), map[string]any{
		"project_id": "start-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("expected set_current_feature from todo to succeed, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-progress") {
		t.Errorf("expected 'in-progress' in result, got:\n%s", text)
	}
}

func TestSetCurrentFeatureFromNeedsEdits(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Restart Test")
	featureID := createTestFeature(t, store, "restart-test", "Restartable Feature")

	// Move through full flow to in-review, then reject.
	startFeature(t, store, "restart-test", featureID)
	advanceWithGateEvidence(t, store, "restart-test", featureID, "in-progress")
	advanceWithGateEvidence(t, store, "restart-test", featureID, "in-testing")
	advanceWithGateEvidence(t, store, "restart-test", featureID, "in-docs")

	// Reject to needs-edits.
	callTool(t, tools.RejectFeature(store), map[string]any{
		"project_id": "restart-test",
		"feature_id": featureID,
		"reason":     "Needs fixes",
	})

	// set_current_feature from needs-edits -> in-progress.
	resp := callTool(t, tools.SetCurrentFeature(store), map[string]any{
		"project_id": "restart-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("expected set_current_feature from needs-edits to succeed, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-progress") {
		t.Errorf("expected 'in-progress' in result, got:\n%s", text)
	}
}

func TestSetCurrentFeatureBlockedFromInProgress(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Block Start Test")
	featureID := createTestFeature(t, store, "block-start-test", "Already Started Feature")

	// Move to in-progress.
	startFeature(t, store, "block-start-test", featureID)

	// Trying set_current_feature again from in-progress should fail.
	resp := callTool(t, tools.SetCurrentFeature(store), map[string]any{
		"project_id": "block-start-test",
		"feature_id": featureID,
	})
	if resp.Success {
		t.Fatal("expected set_current_feature from in-progress to fail")
	}
	if resp.ErrorCode != "workflow_error" {
		t.Errorf("expected workflow_error, got %q", resp.ErrorCode)
	}
}

func TestAdvanceFromInReviewBlocked(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Review Block Test")
	featureID := createTestFeature(t, store, "review-block-test", "Review Block Feature")

	// Move through full workflow to in-review.
	startFeature(t, store, "review-block-test", featureID)
	advanceWithGateEvidence(t, store, "review-block-test", featureID, "in-progress")
	advanceWithGateEvidence(t, store, "review-block-test", featureID, "in-testing")
	advanceWithGateEvidence(t, store, "review-block-test", featureID, "in-docs")

	// advance_feature from in-review should be blocked.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "review-block-test",
		"feature_id": featureID,
		"evidence":   "## Changes\n- some/file.go (trying to self-approve)",
	})
	if resp.Success {
		t.Fatal("expected advance_feature from in-review to be blocked")
	}
	if resp.ErrorCode != "gate_blocked" {
		t.Errorf("expected gate_blocked, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.ErrorMessage, "submit_review") {
		t.Errorf("expected error to mention submit_review, got:\n%s", resp.ErrorMessage)
	}
}

func TestGetGateRequirements(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Gate Req Test")
	featureID := createTestFeature(t, store, "gate-req-test", "Gate Req Feature")

	// From todo -- should mention set_current_feature (no gate, but not "free" via advance).
	resp := callTool(t, tools.GetGateRequirements(store), map[string]any{
		"project_id": "gate-req-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_gate_requirements failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	// todo -> in-progress is not gated, so it should say "free" or mention advance_feature.
	if !strings.Contains(text, "free") && !strings.Contains(text, "advance_feature") {
		t.Errorf("expected 'free' or 'advance_feature' for todo->in-progress, got:\n%s", text)
	}

	// Move to in-progress via set_current_feature.
	startFeature(t, store, "gate-req-test", featureID)

	// From in-progress -- should show Code Complete gate with ## Changes.
	resp = callTool(t, tools.GetGateRequirements(store), map[string]any{
		"project_id": "gate-req-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_gate_requirements failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "Changes") {
		t.Errorf("expected 'Changes' section requirement for in-progress gate, got:\n%s", text)
	}

	// Advance to in-testing.
	advanceWithGateEvidence(t, store, "gate-req-test", featureID, "in-progress")

	// From in-testing -- should show Test Complete gate with ## Results.
	resp = callTool(t, tools.GetGateRequirements(store), map[string]any{
		"project_id": "gate-req-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_gate_requirements failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "Results") {
		t.Errorf("expected 'Results' section requirement for in-testing gate, got:\n%s", text)
	}

	// Advance to in-docs.
	advanceWithGateEvidence(t, store, "gate-req-test", featureID, "in-testing")

	// From in-docs -- should show Docs Complete gate with ## Docs.
	resp = callTool(t, tools.GetGateRequirements(store), map[string]any{
		"project_id": "gate-req-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_gate_requirements failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "Docs") {
		t.Errorf("expected 'Docs' section requirement for in-docs gate, got:\n%s", text)
	}

	// Advance to in-review.
	advanceWithGateEvidence(t, store, "gate-req-test", featureID, "in-docs")

	// From in-review -- should mention submit_review.
	resp = callTool(t, tools.GetGateRequirements(store), map[string]any{
		"project_id": "gate-req-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("get_gate_requirements failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "submit_review") {
		t.Errorf("expected 'submit_review' for in-review status, got:\n%s", text)
	}
	if !strings.Contains(text, "AskUserQuestion") {
		t.Errorf("expected 'AskUserQuestion' instruction for in-review, got:\n%s", text)
	}
}

// TestBugSkipsDocs verifies that bugs skip the docs gate and go from
// in-testing directly to in-review.
func TestBugSkipsDocs(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Bug Skip Test")
	featureID := createTestFeatureWithKind(t, store, "bug-skip-test", "Login Crash", "bug")

	// Move through: todo -> in-progress -> in-testing.
	startFeature(t, store, "bug-skip-test", featureID)
	advanceWithGateEvidence(t, store, "bug-skip-test", featureID, "in-progress")

	// From in-testing, bugs should go to in-review (skipping in-docs).
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "bug-skip-test",
		"feature_id": featureID,
		"evidence":   testCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected bug to advance from in-testing, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-review") {
		t.Errorf("expected bug to skip docs and go to in-review, got:\n%s", text)
	}
}

// TestHotfixSkipsDocs verifies that hotfixes skip the docs gate.
func TestHotfixSkipsDocs(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Hotfix Skip Test")
	featureID := createTestFeatureWithKind(t, store, "hotfix-skip-test", "Urgent Fix", "hotfix")

	// Move through: todo -> in-progress -> in-testing.
	startFeature(t, store, "hotfix-skip-test", featureID)
	advanceWithGateEvidence(t, store, "hotfix-skip-test", featureID, "in-progress")

	// From in-testing, hotfixes should go to in-review (skipping in-docs).
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "hotfix-skip-test",
		"feature_id": featureID,
		"evidence":   testCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected hotfix to advance from in-testing, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-review") {
		t.Errorf("expected hotfix to skip docs and go to in-review, got:\n%s", text)
	}
}

// TestTestcaseSkipsDocs verifies that testcases skip the docs gate.
func TestTestcaseSkipsDocs(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Testcase Skip Test")
	featureID := createTestFeatureWithKind(t, store, "testcase-skip-test", "Auth Tests", "testcase")

	// Move through: todo -> in-progress -> in-testing.
	startFeature(t, store, "testcase-skip-test", featureID)
	advanceWithGateEvidence(t, store, "testcase-skip-test", featureID, "in-progress")

	// From in-testing, testcases should go to in-review (skipping in-docs).
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "testcase-skip-test",
		"feature_id": featureID,
		"evidence":   testCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected testcase to advance from in-testing, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-review") {
		t.Errorf("expected testcase to skip docs and go to in-review, got:\n%s", text)
	}
}

// TestFeatureDoesNotSkipDocs verifies that regular features go through docs.
func TestFeatureDoesNotSkipDocs(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "No Skip Test")
	featureID := createTestFeature(t, store, "no-skip-test", "Regular Feature")

	// Move through: todo -> in-progress -> in-testing.
	startFeature(t, store, "no-skip-test", featureID)
	advanceWithGateEvidence(t, store, "no-skip-test", featureID, "in-progress")

	// From in-testing, regular features should go to in-docs (not skip).
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "no-skip-test",
		"feature_id": featureID,
		"evidence":   testCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected feature to advance from in-testing, got: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "in-docs") {
		t.Errorf("expected regular feature to go to in-docs, got:\n%s", text)
	}
}

// TestGateBlocksWithNoFilePaths verifies that evidence with the right section
// but no file paths is rejected.
func TestGateBlocksWithNoFilePaths(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "No Paths Test")
	featureID := createTestFeature(t, store, "no-paths-test", "No Paths Feature")

	startFeature(t, store, "no-paths-test", featureID)

	// ## Changes present but no file paths in it.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "no-paths-test",
		"feature_id": featureID,
		"evidence":   "## Changes\nMade various improvements to the codebase and refactored the module.",
	})
	if resp.Success {
		t.Fatal("expected gate to block when no file paths in ## Changes")
	}
	if resp.ErrorCode != "gate_blocked" {
		t.Errorf("expected gate_blocked, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.ErrorMessage, "file path") {
		t.Errorf("expected error to mention file paths, got:\n%s", resp.ErrorMessage)
	}
}

// TestDocsGateRequiresDocsFolder verifies that the docs gate requires files
// in the docs/ folder.
func TestDocsGateRequiresDocsFolder(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Docs Folder Test")
	featureID := createTestFeature(t, store, "docs-folder-test", "Docs Folder Feature")

	// Move through to in-docs.
	startFeature(t, store, "docs-folder-test", featureID)
	advanceWithGateEvidence(t, store, "docs-folder-test", featureID, "in-progress")
	advanceWithGateEvidence(t, store, "docs-folder-test", featureID, "in-testing")

	// Try advancing with ## Docs referencing files NOT in docs/ folder.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "docs-folder-test",
		"feature_id": featureID,
		"evidence":   "## Docs\n- src/readme.md (updated readme in source dir)",
	})
	if resp.Success {
		t.Fatal("expected docs gate to reject files not in docs/ folder")
	}
	// Should get needs_approval (file type mismatch) since files exist but wrong folder.
	if resp.ErrorCode != "needs_approval" {
		t.Errorf("expected needs_approval for wrong folder, got %q: %s", resp.ErrorCode, resp.ErrorMessage)
	}
}

// TestAdvanceRequiresEvidenceParameter verifies that advance_feature always
// requires the evidence parameter (it is a required field in the new workflow).
func TestAdvanceRequiresEvidenceParameter(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Ev Required Test")
	featureID := createTestFeature(t, store, "ev-required-test", "Evidence Required Feature")

	startFeature(t, store, "ev-required-test", featureID)

	// Calling advance_feature without evidence should return validation_error.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "ev-required-test",
		"feature_id": featureID,
	})
	if resp.Success {
		t.Fatal("expected advance_feature without evidence to fail")
	}
	if resp.ErrorCode != "validation_error" {
		t.Errorf("expected validation_error, got %q", resp.ErrorCode)
	}
}

// TestSubmitReviewRequiresInReview verifies that submit_review only works from
// the in-review status.
func TestSubmitReviewRequiresInReview(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "Review Status Test")
	featureID := createTestFeature(t, store, "review-status-test", "Review Status Feature")

	// Feature is in todo status -- submit_review should fail.
	resp := callTool(t, tools.SubmitReview(store), map[string]any{
		"project_id": "review-status-test",
		"feature_id": featureID,
		"status":     "approved",
	})
	if resp.Success {
		t.Fatal("expected submit_review from todo to fail")
	}
	if resp.ErrorCode != "workflow_error" {
		t.Errorf("expected workflow_error, got %q", resp.ErrorCode)
	}
}

// TestFullWorkflowEndToEnd runs the complete workflow from creation to done.
func TestFullWorkflowEndToEnd(t *testing.T) {
	store, _ := testEnv()
	createTestProject(t, store, "E2E Test")
	featureID := createTestFeature(t, store, "e2e-test", "End to End Feature")

	// 1. Start: todo -> in-progress.
	startFeature(t, store, "e2e-test", featureID)

	// 2. Code complete: in-progress -> in-testing.
	advanceWithGateEvidence(t, store, "e2e-test", featureID, "in-progress")

	// 3. Tests complete: in-testing -> in-docs.
	advanceWithGateEvidence(t, store, "e2e-test", featureID, "in-testing")

	// 4. Docs complete: in-docs -> in-review.
	advanceWithGateEvidence(t, store, "e2e-test", featureID, "in-docs")

	// 5. Approve: in-review -> done.
	resp := callTool(t, tools.SubmitReview(store), map[string]any{
		"project_id": "e2e-test",
		"feature_id": featureID,
		"status":     "approved",
		"comment":    "All good, shipping it.",
	})
	if !resp.Success {
		t.Fatalf("submit_review failed: %s", resp.ErrorMessage)
	}
	text := resultText(t, resp)
	if !strings.Contains(text, "done") {
		t.Errorf("expected 'done' in final result, got:\n%s", text)
	}

	// Verify progress shows 100%.
	resp = callTool(t, tools.GetProgress(store), map[string]any{
		"project_id": "e2e-test",
	})
	if !resp.Success {
		t.Fatalf("get_progress failed: %s", resp.ErrorMessage)
	}
	text = resultText(t, resp)
	if !strings.Contains(text, "100.0%") {
		t.Errorf("expected '100.0%%' completion after done, got:\n%s", text)
	}
}
