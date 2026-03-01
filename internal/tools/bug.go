package tools

import (
	"context"
	"fmt"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreateBugReportSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project slug"},
			"title":           map[string]any{"type": "string", "description": "Bug title"},
			"description":     map[string]any{"type": "string", "description": "Bug description"},
			"related_feature": map[string]any{"type": "string", "description": "Feature ID that caused the bug (optional)"},
			"priority":        map[string]any{"type": "string", "description": "Priority (P0-P3)", "enum": []any{"P0", "P1", "P2", "P3"}},
		},
		"required": []any{"project_id", "title"},
	})
	return s
}

// ---------- Handlers ----------

// CreateBugReport creates a new bug report in a project. Bugs use Kind=bug
// and start in backlog status. An optional related_feature links the bug to
// the feature that caused it.
func CreateBugReport(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		title := helpers.GetString(req.Arguments, "title")
		description := helpers.GetString(req.Arguments, "description")
		relatedFeature := helpers.GetString(req.Arguments, "related_feature")
		priority := helpers.GetStringOr(req.Arguments, "priority", "P1")

		// Validate priority.
		if err := helpers.ValidateOneOf(priority, "P0", "P1", "P2", "P3"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		featureID := helpers.NewFeatureID()
		now := helpers.NowISO()

		var labels []string
		if relatedFeature != "" {
			labels = append(labels, fmt.Sprintf("reported-against:%s", relatedFeature))
		}

		feat := &types.FeatureData{
			ID:          featureID,
			ProjectID:   projectID,
			Title:       title,
			Description: description,
			Status:      types.StatusBacklog,
			Priority:    priority,
			Kind:        types.KindBug,
			Labels:      labels,
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n%s\n", title, description)
		if relatedFeature != "" {
			body += fmt.Sprintf("\nReported against feature %s\n", relatedFeature)
		}

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created bug **%s**: %s\n\n%s", featureID, title, helpers.FormatFeatureMD(feat))
		return helpers.TextResult(md), nil
	}
}
