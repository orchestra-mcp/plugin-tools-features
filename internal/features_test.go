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

	if !strings.Contains(text, "backlog") {
		t.Errorf("expected status backlog in result, got:\n%s", text)
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

	// Expected workflow path: backlog -> todo -> in-progress -> ready-for-testing
	// -> in-testing -> ready-for-docs -> in-docs -> documented -> in-review -> done
	expectedTransitions := []struct {
		from string
		to   string
	}{
		{"backlog", "todo"},
		{"todo", "in-progress"},
		{"in-progress", "ready-for-testing"},
		{"ready-for-testing", "in-testing"},
		{"in-testing", "ready-for-docs"},
		{"ready-for-docs", "in-docs"},
		{"in-docs", "documented"},
		{"documented", "in-review"},
		{"in-review", "done"},
	}

	for _, tt := range expectedTransitions {
		resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
			"project_id": "workflow-test",
			"feature_id": featureID,
			"evidence":   "Completed " + tt.from,
		})
		if !resp.Success {
			t.Fatalf("advance from %s failed: %s", tt.from, resp.ErrorMessage)
		}
		text := resultText(t, resp)
		if !strings.Contains(text, tt.from) {
			t.Errorf("expected from %q in result, got:\n%s", tt.from, text)
		}
		if !strings.Contains(text, tt.to) {
			t.Errorf("expected to %q in result, got:\n%s", tt.to, text)
		}
	}

	// Advancing from done should fail.
	resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "workflow-test",
		"feature_id": featureID,
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

	// Advance to in-review: backlog -> todo -> in-progress -> ready-for-testing ->
	// in-testing -> ready-for-docs -> in-docs -> documented -> in-review
	for i := 0; i < 8; i++ {
		resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
			"project_id": "reject-test",
			"feature_id": featureID,
		})
		if !resp.Success {
			t.Fatalf("advance step %d failed: %s", i, resp.ErrorMessage)
		}
	}

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

	// From needs-edits, can go back to in-progress.
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
	id1 := createTestFeature(t, store, "next-test", "Low Priority Feature")
	id2 := createTestFeature(t, store, "next-test", "High Priority Feature")

	// Set one feature to P0.
	callTool(t, tools.UpdateFeature(store), map[string]any{
		"project_id": "next-test",
		"feature_id": id2,
		"priority":   "P0",
	})

	// Advance both to todo (from backlog).
	callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "next-test",
		"feature_id": id1,
	})
	callTool(t, tools.AdvanceFeature(store), map[string]any{
		"project_id": "next-test",
		"feature_id": id2,
	})

	// Get next feature should return the P0 one.
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

	// Advance to documented (7 transitions).
	for i := 0; i < 7; i++ {
		resp := callTool(t, tools.AdvanceFeature(store), map[string]any{
			"project_id": "review-test",
			"feature_id": featureID,
		})
		if !resp.Success {
			t.Fatalf("advance step %d failed: %s", i, resp.ErrorMessage)
		}
	}

	// Request review.
	resp := callTool(t, tools.RequestReview(store), map[string]any{
		"project_id": "review-test",
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("request_review failed: %s", resp.ErrorMessage)
	}

	// Check pending reviews.
	resp = callTool(t, tools.GetPendingReviews(store), map[string]any{
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
