package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreateTestCaseSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project slug"},
			"title":           map[string]any{"type": "string", "description": "Test case title"},
			"description":     map[string]any{"type": "string", "description": "Test case description"},
			"related_feature": map[string]any{"type": "string", "description": "Feature ID this test case covers (required)"},
			"priority":        map[string]any{"type": "string", "description": "Priority (P0-P3)", "enum": []any{"P0", "P1", "P2", "P3"}},
		},
		"required": []any{"project_id", "title", "related_feature"},
	})
	return s
}

func BulkCreateTestCasesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project slug"},
			"related_feature": map[string]any{"type": "string", "description": "Feature ID these test cases cover"},
			"test_cases": map[string]any{
				"type":        "string",
				"description": "JSON array of test case objects: [{\"title\": \"...\", \"description\": \"...\", \"priority\": \"P1\"}]. Only title is required per entry.",
			},
		},
		"required": []any{"project_id", "related_feature", "test_cases"},
	})
	return s
}

// ---------- Handlers ----------

// CreateTestCase creates a new test case linked to a feature. Test cases use
// Kind=testcase and start in backlog status. The related_feature is stored as
// a label "test-for:{feature_id}".
func CreateTestCase(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title", "related_feature"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		title := helpers.GetString(req.Arguments, "title")
		description := helpers.GetString(req.Arguments, "description")
		relatedFeature := helpers.GetString(req.Arguments, "related_feature")
		priority := helpers.GetStringOr(req.Arguments, "priority", "P1")

		if err := helpers.ValidateOneOf(priority, "P0", "P1", "P2", "P3"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		// Verify the related feature exists.
		_, _, _, err = store.ReadFeature(ctx, projectID, relatedFeature)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("related feature %q not found", relatedFeature)), nil
		}

		featureID := helpers.NewFeatureID()
		now := helpers.NowISO()

		labels := []string{fmt.Sprintf("test-for:%s", relatedFeature)}

		feat := &types.FeatureData{
			ID:          featureID,
			ProjectID:   projectID,
			Title:       title,
			Description: description,
			Status:      types.StatusBacklog,
			Priority:    priority,
			Kind:        types.KindTestcase,
			Labels:      labels,
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n%s\n\nTest case for feature %s\n", title, description, relatedFeature)

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		applyAutoAssignment(ctx, store, projectID, featureID, string(types.KindTestcase))

		md := fmt.Sprintf("Created test case **%s**: %s\n\n%s", featureID, title, helpers.FormatFeatureMD(feat))
		return helpers.TextResult(md), nil
	}
}

// testCaseEntry is used for parsing the JSON array in bulk_create_test_cases.
type testCaseEntry struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

// BulkCreateTestCases creates multiple test cases for a feature in one call.
func BulkCreateTestCases(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "related_feature", "test_cases"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		relatedFeature := helpers.GetString(req.Arguments, "related_feature")
		testCasesJSON := helpers.GetString(req.Arguments, "test_cases")

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		// Verify the related feature exists.
		_, _, _, err = store.ReadFeature(ctx, projectID, relatedFeature)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("related feature %q not found", relatedFeature)), nil
		}

		// Parse JSON array.
		var entries []testCaseEntry
		if err := json.Unmarshal([]byte(testCasesJSON), &entries); err != nil {
			return helpers.ErrorResult("validation_error", fmt.Sprintf("invalid test_cases JSON: %v", err)), nil
		}
		if len(entries) == 0 {
			return helpers.ErrorResult("validation_error", "test_cases array must not be empty"), nil
		}

		now := helpers.NowISO()
		labels := []string{fmt.Sprintf("test-for:%s", relatedFeature)}
		var created []string

		for _, entry := range entries {
			if entry.Title == "" {
				continue // skip entries without a title
			}
			priority := entry.Priority
			if priority == "" {
				priority = "P1"
			}

			featureID := helpers.NewFeatureID()
			feat := &types.FeatureData{
				ID:          featureID,
				ProjectID:   projectID,
				Title:       entry.Title,
				Description: entry.Description,
				Status:      types.StatusBacklog,
				Priority:    priority,
				Kind:        types.KindTestcase,
				Labels:      labels,
				Version:     0,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			body := fmt.Sprintf("# %s\n\n%s\n\nTest case for feature %s\n", entry.Title, entry.Description, relatedFeature)

			_, err := store.WriteFeature(ctx, projectID, featureID, feat, body, 0)
			if err != nil {
				continue // skip failed writes
			}

			applyAutoAssignment(ctx, store, projectID, featureID, string(types.KindTestcase))
			created = append(created, featureID)
		}

		if len(created) == 0 {
			return helpers.ErrorResult("storage_error", "no test cases were created"), nil
		}

		md := fmt.Sprintf("Created **%d** test cases for %s:\n\n%s",
			len(created), relatedFeature, strings.Join(created, ", "))
		return helpers.TextResult(md), nil
	}
}
