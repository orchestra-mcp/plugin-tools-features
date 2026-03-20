package tools

import (
	"context"
	"regexp"
	"strings"
	"testing"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/workflow"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Test helpers (mirrors internal/features_test.go) ----------

func delTestEnv() (*storage.FeatureStorage, context.Context) {
	mem := storage.NewInMemoryStorage()
	store := storage.NewFeatureStorage(mem)
	return store, context.Background()
}

func delCallTool(t *testing.T, handler ToolHandler, args map[string]any) *pluginv1.ToolResponse {
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

func delResultText(t *testing.T, resp *pluginv1.ToolResponse) string {
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

var delFeatureIDPattern = regexp.MustCompile(`FEAT-[A-Z0-9]+`)
var delPersonIDPattern = regexp.MustCompile(`PERS-[A-Z0-9]+`)
var delDelegationIDPattern = regexp.MustCompile(`DEL-[A-Z0-9]+`)

func delCreateProject(t *testing.T, store *storage.FeatureStorage, name string) {
	t.Helper()
	resp := delCallTool(t, CreateProject(store), map[string]any{
		"name": name,
	})
	if !resp.Success {
		t.Fatalf("create_project failed: %s", resp.ErrorMessage)
	}
}

func delCreateFeature(t *testing.T, store *storage.FeatureStorage, projectID, title string) string {
	t.Helper()
	resp := delCallTool(t, CreateFeature(store), map[string]any{
		"project_id": projectID,
		"title":      title,
	})
	if !resp.Success {
		t.Fatalf("create_feature failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	id := delFeatureIDPattern.FindString(text)
	if id == "" {
		t.Fatal("create_feature did not return a feature ID in the text")
	}
	return id
}

func delCreatePerson(t *testing.T, store *storage.FeatureStorage, projectID, name, role string) string {
	t.Helper()
	resp := delCallTool(t, CreatePerson(store), map[string]any{
		"project_id": projectID,
		"name":       name,
		"role":       role,
	})
	if !resp.Success {
		t.Fatalf("create_person failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	id := delPersonIDPattern.FindString(text)
	if id == "" {
		t.Fatal("create_person did not return a person ID in the text")
	}
	return id
}

func delStartFeature(t *testing.T, store *storage.FeatureStorage, projectID, featureID string) {
	t.Helper()
	eng := workflow.DefaultEngine()
	resolver := workflow.NewResolver(eng)
	resp := delCallTool(t, SetCurrentFeature(store, resolver), map[string]any{
		"project_id": projectID,
		"feature_id": featureID,
	})
	if !resp.Success {
		t.Fatalf("set_current_feature failed: %s", resp.ErrorMessage)
	}
}

// Code Complete gate evidence (in-progress -> in-testing).
const delCodeCompleteEvidence = "## Changes\n- src/handler.go (added new endpoint for authentication)\n- src/service.go (business logic for token validation)\n- src/models.go (new user model fields)"

// delForceInProgress reads a feature and directly sets its status to in-progress
// via the storage layer, bypassing the WIP limit check in SetCurrentFeature.
// This is useful when tests need multiple features in-progress simultaneously.
func delForceInProgress(t *testing.T, store *storage.FeatureStorage, ctx context.Context, projectID, featureID string) {
	t.Helper()
	feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
	if err != nil {
		t.Fatalf("read feature %s failed: %v", featureID, err)
	}
	feat.Status = "in-progress"
	_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
	if err != nil {
		t.Fatalf("write feature %s to in-progress failed: %v", featureID, err)
	}
}

// ---------- Tests ----------

func TestDelegateFeature_Success(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Deleg Success")
	featureID := delCreateFeature(t, store, "deleg-success", "Delegatable Feature")
	personID := delCreatePerson(t, store, "deleg-success", "Alice Smith", "reviewer")

	// Move feature to in-progress.
	delStartFeature(t, store, "deleg-success", featureID)

	// Delegate from the feature to the person.
	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "deleg-success",
		"feature_id": featureID,
		"to_person":  personID,
		"question":   "Should we use JWT or session-based auth?",
		"context":    "We need to decide before implementing the login flow.",
	})
	if !resp.Success {
		t.Fatalf("delegate_feature failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)

	// Verify delegation was created.
	delegationID := delDelegationIDPattern.FindString(text)
	if delegationID == "" {
		t.Fatal("delegate_feature did not return a delegation ID")
	}
	if !strings.Contains(text, "blocked") {
		t.Errorf("expected 'blocked' in result, got:\n%s", text)
	}
	if !strings.Contains(text, personID) {
		t.Errorf("expected person ID %s in result, got:\n%s", personID, text)
	}

	// Verify the feature now has the delegation label.
	featResp := delCallTool(t, GetFeature(store), map[string]any{
		"project_id": "deleg-success",
		"feature_id": featureID,
	})
	featText := delResultText(t, featResp)
	if !strings.Contains(featText, "delegation:"+delegationID) {
		t.Errorf("expected delegation label 'delegation:%s' on feature, got:\n%s", delegationID, featText)
	}
}

func TestDelegateFeature_FeatureNotFound(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Deleg No Feat")
	personID := delCreatePerson(t, store, "deleg-no-feat", "Bob Jones", "developer")

	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "deleg-no-feat",
		"feature_id": "FEAT-NONEXISTENT",
		"to_person":  personID,
		"question":   "Does this work?",
	})
	if resp.Success {
		t.Fatal("expected delegate_feature to fail for non-existent feature")
	}
	if resp.ErrorCode != "not_found" {
		t.Errorf("expected not_found error, got %q", resp.ErrorCode)
	}
}

func TestDelegateFeature_PersonNotFound(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Deleg No Person")
	featureID := delCreateFeature(t, store, "deleg-no-person", "Feature Without Person")

	// Move feature to in-progress.
	delStartFeature(t, store, "deleg-no-person", featureID)

	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "deleg-no-person",
		"feature_id": featureID,
		"to_person":  "PERS-NONEXISTENT",
		"question":   "Who are you?",
	})
	if resp.Success {
		t.Fatal("expected delegate_feature to fail for non-existent person")
	}
	if resp.ErrorCode != "not_found" {
		t.Errorf("expected not_found error, got %q", resp.ErrorCode)
	}
}

func TestDelegateFeature_FeatureNotInProgress(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Deleg Wrong Status")
	featureID := delCreateFeature(t, store, "deleg-wrong-status", "Todo Feature")
	personID := delCreatePerson(t, store, "deleg-wrong-status", "Carol White", "lead")

	// Feature is in todo status (not in-progress).
	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "deleg-wrong-status",
		"feature_id": featureID,
		"to_person":  personID,
		"question":   "Can we delegate from todo?",
	})
	if resp.Success {
		t.Fatal("expected delegate_feature to fail when feature is not in-progress")
	}
	if resp.ErrorCode != "invalid_status" {
		t.Errorf("expected invalid_status error, got %q", resp.ErrorCode)
	}
}

func TestRespondDelegation_Success(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Respond Success")
	featureID := delCreateFeature(t, store, "respond-success", "Respondable Feature")
	personID := delCreatePerson(t, store, "respond-success", "Dave Brown", "reviewer")

	// Move feature to in-progress.
	delStartFeature(t, store, "respond-success", featureID)

	// Create delegation.
	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "respond-success",
		"feature_id": featureID,
		"to_person":  personID,
		"question":   "Which database should we use?",
	})
	if !resp.Success {
		t.Fatalf("delegate_feature failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	delegationID := delDelegationIDPattern.FindString(text)
	if delegationID == "" {
		t.Fatal("delegate_feature did not return a delegation ID")
	}

	// Respond to delegation.
	resp = delCallTool(t, RespondDelegation(store), map[string]any{
		"project_id":    "respond-success",
		"delegation_id": delegationID,
		"response":      "Use PostgreSQL with pgvector.",
	})
	if !resp.Success {
		t.Fatalf("respond_delegation failed: %s", resp.ErrorMessage)
	}
	text = delResultText(t, resp)
	if !strings.Contains(text, "unblocked") {
		t.Errorf("expected 'unblocked' in response result, got:\n%s", text)
	}
	if !strings.Contains(text, "PostgreSQL") {
		t.Errorf("expected response text in result, got:\n%s", text)
	}

	// Verify delegation status is answered.
	resp = delCallTool(t, GetDelegation(store), map[string]any{
		"project_id":    "respond-success",
		"delegation_id": delegationID,
	})
	if !resp.Success {
		t.Fatalf("get_delegation failed: %s", resp.ErrorMessage)
	}
	text = delResultText(t, resp)
	if !strings.Contains(text, "answered") {
		t.Errorf("expected 'answered' status in delegation, got:\n%s", text)
	}

	// Verify feature no longer has the delegation label.
	featResp := delCallTool(t, GetFeature(store), map[string]any{
		"project_id": "respond-success",
		"feature_id": featureID,
	})
	featText := delResultText(t, featResp)
	if strings.Contains(featText, "delegation:"+delegationID) {
		t.Errorf("expected delegation label removed from feature after response, got:\n%s", featText)
	}
}

func TestRespondDelegation_AlreadyAnswered(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Respond Twice")
	featureID := delCreateFeature(t, store, "respond-twice", "Double Response Feature")
	personID := delCreatePerson(t, store, "respond-twice", "Eve Green", "developer")

	// Move feature to in-progress.
	delStartFeature(t, store, "respond-twice", featureID)

	// Create delegation.
	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "respond-twice",
		"feature_id": featureID,
		"to_person":  personID,
		"question":   "Should we use REST or GraphQL?",
	})
	if !resp.Success {
		t.Fatalf("delegate_feature failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	delegationID := delDelegationIDPattern.FindString(text)

	// First response.
	resp = delCallTool(t, RespondDelegation(store), map[string]any{
		"project_id":    "respond-twice",
		"delegation_id": delegationID,
		"response":      "Use REST.",
	})
	if !resp.Success {
		t.Fatalf("first respond_delegation failed: %s", resp.ErrorMessage)
	}

	// Second response should fail.
	resp = delCallTool(t, RespondDelegation(store), map[string]any{
		"project_id":    "respond-twice",
		"delegation_id": delegationID,
		"response":      "Actually, use GraphQL.",
	})
	if resp.Success {
		t.Fatal("expected second respond_delegation to fail")
	}
	if resp.ErrorCode != "invalid_status" {
		t.Errorf("expected invalid_status error, got %q", resp.ErrorCode)
	}
}

func TestListDelegations(t *testing.T) {
	store, ctx := delTestEnv()
	delCreateProject(t, store, "List Delegs")
	feat1 := delCreateFeature(t, store, "list-delegs", "Feature One")
	feat2 := delCreateFeature(t, store, "list-delegs", "Feature Two")
	person1 := delCreatePerson(t, store, "list-delegs", "Frank Lee", "reviewer")
	person2 := delCreatePerson(t, store, "list-delegs", "Grace Kim", "lead")

	// Move features to in-progress. Use direct store write for the second
	// feature to bypass the one-at-a-time WIP limit in SetCurrentFeature.
	delStartFeature(t, store, "list-delegs", feat1)
	delForceInProgress(t, store, ctx, "list-delegs", feat2)

	// Create 3 delegations.
	resp1 := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "list-delegs",
		"feature_id": feat1,
		"to_person":  person1,
		"question":   "Question 1?",
	})
	if !resp1.Success {
		t.Fatalf("delegate 1 failed: %s", resp1.ErrorMessage)
	}
	delID1 := delDelegationIDPattern.FindString(delResultText(t, resp1))

	resp2 := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "list-delegs",
		"feature_id": feat2,
		"to_person":  person2,
		"question":   "Question 2?",
	})
	if !resp2.Success {
		t.Fatalf("delegate 2 failed: %s", resp2.ErrorMessage)
	}

	resp3 := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "list-delegs",
		"feature_id": feat1,
		"to_person":  person2,
		"question":   "Question 3?",
	})
	if !resp3.Success {
		t.Fatalf("delegate 3 failed: %s", resp3.ErrorMessage)
	}

	// List all delegations.
	resp := delCallTool(t, ListDelegations(store), map[string]any{
		"project_id": "list-delegs",
	})
	if !resp.Success {
		t.Fatalf("list_delegations failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	if !strings.Contains(text, "3") {
		t.Errorf("expected count 3 in list header, got:\n%s", text)
	}

	// Filter by feature_id — feat1 has 2 delegations, feat2 has 1.
	t.Run("filter_by_feature_id", func(t *testing.T) {
		resp := delCallTool(t, ListDelegations(store), map[string]any{
			"project_id": "list-delegs",
			"feature_id": feat1,
		})
		if !resp.Success {
			t.Fatalf("list_delegations (feature_id=%s) failed: %s", feat1, resp.ErrorMessage)
		}
		text := delResultText(t, resp)
		if !strings.Contains(text, "(2)") {
			t.Errorf("expected 2 delegations for feature %s, got:\n%s", feat1, text)
		}

		resp = delCallTool(t, ListDelegations(store), map[string]any{
			"project_id": "list-delegs",
			"feature_id": feat2,
		})
		if !resp.Success {
			t.Fatalf("list_delegations (feature_id=%s) failed: %s", feat2, resp.ErrorMessage)
		}
		text = delResultText(t, resp)
		if !strings.Contains(text, "(1)") {
			t.Errorf("expected 1 delegation for feature %s, got:\n%s", feat2, text)
		}
	})

	// Filter by person_id — person1 is involved in 1 delegation (to),
	// person2 is involved in 2 delegations (to).
	t.Run("filter_by_person_id", func(t *testing.T) {
		resp := delCallTool(t, ListDelegations(store), map[string]any{
			"project_id": "list-delegs",
			"person_id":  person1,
		})
		if !resp.Success {
			t.Fatalf("list_delegations (person_id=%s) failed: %s", person1, resp.ErrorMessage)
		}
		text := delResultText(t, resp)
		if !strings.Contains(text, "(1)") {
			t.Errorf("expected 1 delegation involving person %s, got:\n%s", person1, text)
		}

		resp = delCallTool(t, ListDelegations(store), map[string]any{
			"project_id": "list-delegs",
			"person_id":  person2,
		})
		if !resp.Success {
			t.Fatalf("list_delegations (person_id=%s) failed: %s", person2, resp.ErrorMessage)
		}
		text = delResultText(t, resp)
		if !strings.Contains(text, "(2)") {
			t.Errorf("expected 2 delegations involving person %s, got:\n%s", person2, text)
		}
	})

	// Answer one delegation, then filter by status.
	delCallTool(t, RespondDelegation(store), map[string]any{
		"project_id":    "list-delegs",
		"delegation_id": delID1,
		"response":      "Answered.",
	})

	t.Run("filter_by_status_pending", func(t *testing.T) {
		resp := delCallTool(t, ListDelegations(store), map[string]any{
			"project_id": "list-delegs",
			"status":     "pending",
		})
		if !resp.Success {
			t.Fatalf("list_delegations (pending) failed: %s", resp.ErrorMessage)
		}
		text := delResultText(t, resp)
		if !strings.Contains(text, "(2)") {
			t.Errorf("expected 2 pending delegations, got:\n%s", text)
		}
	})

	t.Run("filter_by_status_answered", func(t *testing.T) {
		resp := delCallTool(t, ListDelegations(store), map[string]any{
			"project_id": "list-delegs",
			"status":     "answered",
		})
		if !resp.Success {
			t.Fatalf("list_delegations (answered) failed: %s", resp.ErrorMessage)
		}
		text := delResultText(t, resp)
		if !strings.Contains(text, "(1)") {
			t.Errorf("expected 1 answered delegation, got:\n%s", text)
		}
	})
}

func TestGetPendingDelegations(t *testing.T) {
	store, ctx := delTestEnv()
	delCreateProject(t, store, "Pending Delegs")
	feat1 := delCreateFeature(t, store, "pending-delegs", "Feature Alpha")
	feat2 := delCreateFeature(t, store, "pending-delegs", "Feature Beta")
	personA := delCreatePerson(t, store, "pending-delegs", "Hannah Ryu", "reviewer")
	personB := delCreatePerson(t, store, "pending-delegs", "Ivan Chen", "lead")

	// Move features to in-progress. Use direct store write for the second
	// feature to bypass the one-at-a-time WIP limit in SetCurrentFeature.
	delStartFeature(t, store, "pending-delegs", feat1)
	delForceInProgress(t, store, ctx, "pending-delegs", feat2)

	// Create delegations: 2 to person A, 1 to person B.
	delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "pending-delegs",
		"feature_id": feat1,
		"to_person":  personA,
		"question":   "Question for A (1)?",
	})
	delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "pending-delegs",
		"feature_id": feat2,
		"to_person":  personA,
		"question":   "Question for A (2)?",
	})
	delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "pending-delegs",
		"feature_id": feat1,
		"to_person":  personB,
		"question":   "Question for B?",
	})

	// Get pending for person A.
	resp := delCallTool(t, GetPendingDelegations(store), map[string]any{
		"project_id": "pending-delegs",
		"person_id":  personA,
	})
	if !resp.Success {
		t.Fatalf("get_pending_delegations for person A failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	if !strings.Contains(text, "2") {
		t.Errorf("expected 2 pending delegations for person A, got:\n%s", text)
	}

	// Get pending for person B.
	resp = delCallTool(t, GetPendingDelegations(store), map[string]any{
		"project_id": "pending-delegs",
		"person_id":  personB,
	})
	if !resp.Success {
		t.Fatalf("get_pending_delegations for person B failed: %s", resp.ErrorMessage)
	}
	text = delResultText(t, resp)
	if !strings.Contains(text, "1") {
		t.Errorf("expected 1 pending delegation for person B, got:\n%s", text)
	}

	// Get pending for a person with no delegations.
	resp = delCallTool(t, GetPendingDelegations(store), map[string]any{
		"project_id": "pending-delegs",
		"person_id":  "PERS-NOBODY",
	})
	if !resp.Success {
		t.Fatalf("get_pending_delegations for nobody failed: %s", resp.ErrorMessage)
	}
	text = delResultText(t, resp)
	if !strings.Contains(text, "No delegations found") {
		t.Errorf("expected 'No delegations found' for unknown person, got:\n%s", text)
	}
}

func TestDelegationBlocksAdvance(t *testing.T) {
	store, _ := delTestEnv()
	eng := workflow.DefaultEngine()
	resolver := workflow.NewResolver(eng)
	delCreateProject(t, store, "Deleg Blocks Advance")
	featureID := delCreateFeature(t, store, "deleg-blocks-advance", "Blocked Feature")
	personID := delCreatePerson(t, store, "deleg-blocks-advance", "Jake Moss", "reviewer")

	// Move feature to in-progress.
	delStartFeature(t, store, "deleg-blocks-advance", featureID)

	// Create a delegation to block the feature.
	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "deleg-blocks-advance",
		"feature_id": featureID,
		"to_person":  personID,
		"question":   "Should we proceed with this approach?",
	})
	if !resp.Success {
		t.Fatalf("delegate_feature failed: %s", resp.ErrorMessage)
	}

	// Try to advance the feature — should be blocked by delegation.
	resp = delCallTool(t, AdvanceFeature(store, resolver), map[string]any{
		"project_id": "deleg-blocks-advance",
		"feature_id": featureID,
		"evidence":   delCodeCompleteEvidence,
	})
	if resp.Success {
		t.Fatal("expected advance_feature to fail due to pending delegation")
	}
	if resp.ErrorCode != "delegation_blocked" {
		t.Errorf("expected delegation_blocked error, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.ErrorMessage, personID) {
		t.Errorf("expected person ID %s in error message, got:\n%s", personID, resp.ErrorMessage)
	}
}

func TestDelegationUnblocksAfterResponse(t *testing.T) {
	store, _ := delTestEnv()
	delCreateProject(t, store, "Deleg Unblocks")
	featureID := delCreateFeature(t, store, "deleg-unblocks", "Unblockable Feature")
	personID := delCreatePerson(t, store, "deleg-unblocks", "Kira Tanaka", "lead")

	// Move feature to in-progress.
	delStartFeature(t, store, "deleg-unblocks", featureID)

	// Create delegation.
	resp := delCallTool(t, DelegateFeature(store), map[string]any{
		"project_id": "deleg-unblocks",
		"feature_id": featureID,
		"to_person":  personID,
		"question":   "Is this architecture sound?",
	})
	if !resp.Success {
		t.Fatalf("delegate_feature failed: %s", resp.ErrorMessage)
	}
	text := delResultText(t, resp)
	delegationID := delDelegationIDPattern.FindString(text)
	if delegationID == "" {
		t.Fatal("delegate_feature did not return a delegation ID")
	}

	// Verify the delegation label exists on the feature.
	featResp := delCallTool(t, GetFeature(store), map[string]any{
		"project_id": "deleg-unblocks",
		"feature_id": featureID,
	})
	featText := delResultText(t, featResp)
	if !strings.Contains(featText, "delegation:"+delegationID) {
		t.Errorf("expected delegation label on feature before response, got:\n%s", featText)
	}

	// Respond to delegation.
	resp = delCallTool(t, RespondDelegation(store), map[string]any{
		"project_id":    "deleg-unblocks",
		"delegation_id": delegationID,
		"response":      "Yes, the architecture looks good.",
	})
	if !resp.Success {
		t.Fatalf("respond_delegation failed: %s", resp.ErrorMessage)
	}

	// Verify delegation label is removed from the feature.
	featResp = delCallTool(t, GetFeature(store), map[string]any{
		"project_id": "deleg-unblocks",
		"feature_id": featureID,
	})
	featText = delResultText(t, featResp)
	if strings.Contains(featText, "delegation:"+delegationID) {
		t.Errorf("expected delegation label removed from feature after response, got:\n%s", featText)
	}

	// Now advance_feature should succeed (no longer blocked by delegation).
	eng := workflow.DefaultEngine()
	resolver := workflow.NewResolver(eng)
	resp = delCallTool(t, AdvanceFeature(store, resolver), map[string]any{
		"project_id": "deleg-unblocks",
		"feature_id": featureID,
		"evidence":   delCodeCompleteEvidence,
	})
	if !resp.Success {
		t.Fatalf("expected advance_feature to succeed after delegation was answered, but got error: %s (code: %s)",
			resp.ErrorMessage, resp.ErrorCode)
	}
	advText := delResultText(t, resp)
	if !strings.Contains(advText, "in-testing") {
		t.Errorf("expected feature to advance to in-testing, got:\n%s", advText)
	}
}
