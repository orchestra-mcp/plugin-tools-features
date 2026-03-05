package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreateFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"title":       map[string]any{"type": "string", "description": "Feature title"},
			"description": map[string]any{"type": "string", "description": "Feature description"},
			"priority":    map[string]any{"type": "string", "description": "Priority (P0-P3)", "enum": []any{"P0", "P1", "P2", "P3"}},
			"kind":        map[string]any{"type": "string", "description": "Feature kind", "enum": []any{"feature", "bug", "hotfix", "chore", "testcase"}},
		},
		"required": []any{"project_id", "title"},
	})
	return s
}

func GetFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID (e.g., FEAT-ABC)"},
		},
		"required": []any{"project_id", "feature_id"},
	})
	return s
}

func UpdateFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"feature_id":  map[string]any{"type": "string", "description": "Feature ID"},
			"title":       map[string]any{"type": "string", "description": "New title"},
			"description": map[string]any{"type": "string", "description": "New description"},
			"priority":    map[string]any{"type": "string", "description": "New priority (P0-P3)"},
		},
		"required": []any{"project_id", "feature_id"},
	})
	return s
}

func ListFeaturesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)"},
			"kind":       map[string]any{"type": "string", "description": "Filter by kind (optional)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func DeleteFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
		},
		"required": []any{"project_id", "feature_id"},
	})
	return s
}

func SearchFeaturesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"query":      map[string]any{"type": "string", "description": "Search query"},
			"kind":       map[string]any{"type": "string", "description": "Filter by kind (optional)"},
		},
		"required": []any{"project_id", "query"},
	})
	return s
}

// ---------- Handlers ----------

// CreateFeature creates a new feature in a project.
func CreateFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		title := helpers.GetString(req.Arguments, "title")
		description := helpers.GetString(req.Arguments, "description")
		priority := helpers.GetStringOr(req.Arguments, "priority", "P2")

		// Validate priority.
		if err := helpers.ValidateOneOf(priority, "P0", "P1", "P2", "P3"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		kind := helpers.GetStringOr(req.Arguments, "kind", "feature")
		if err := helpers.ValidateOneOf(kind, types.ValidKinds...); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		featureID := helpers.NewFeatureID()
		now := helpers.NowISO()

		feat := &types.FeatureData{
			ID:          featureID,
			ProjectID:   projectID,
			Title:       title,
			Description: description,
			Status:      types.StatusBacklog,
			Priority:    priority,
			Kind:        types.FeatureKind(kind),
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n%s\n", title, description)

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		applyAutoAssignment(ctx, store, projectID, featureID, kind)

		md := fmt.Sprintf("Created **%s**: %s\n\n%s", featureID, title, helpers.FormatFeatureMD(feat))
		return helpers.TextResult(md), nil
	}
}

// GetFeature returns a feature's data and body.
func GetFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")

		feat, body, _, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		md := helpers.FormatFeatureMD(feat) + "\n---\n\n" + body
		return helpers.TextResult(md), nil
	}
}

// UpdateFeature updates mutable fields of a feature.
func UpdateFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if t := helpers.GetString(req.Arguments, "title"); t != "" {
			feat.Title = t
		}
		if d := helpers.GetString(req.Arguments, "description"); d != "" {
			feat.Description = d
		}
		if p := helpers.GetString(req.Arguments, "priority"); p != "" {
			if err := helpers.ValidateOneOf(p, "P0", "P1", "P2", "P3"); err != nil {
				return helpers.ErrorResult("validation_error", err.Error()), nil
			}
			feat.Priority = p
		}

		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Updated **%s**\n\n%s", featureID, helpers.FormatFeatureMD(feat))
		return helpers.TextResult(md), nil
	}
}

// ListFeatures returns all features in a project, optionally filtered by status.
func ListFeatures(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if statusFilter != "" {
			var filtered []*types.FeatureData
			for _, f := range features {
				if string(f.Status) == statusFilter {
					filtered = append(filtered, f)
				}
			}
			features = filtered
		}

		kindFilter := helpers.GetString(req.Arguments, "kind")
		if kindFilter != "" {
			var filtered []*types.FeatureData
			for _, f := range features {
				k := string(f.Kind)
				if k == "" {
					k = "feature"
				}
				if k == kindFilter {
					filtered = append(filtered, f)
				}
			}
			features = filtered
		}

		if features == nil {
			features = []*types.FeatureData{}
		}

		header := "Features"
		if statusFilter != "" && kindFilter != "" {
			header = fmt.Sprintf("Features (%s, kind=%s)", statusFilter, kindFilter)
		} else if statusFilter != "" {
			header = fmt.Sprintf("Features (%s)", statusFilter)
		} else if kindFilter != "" {
			header = fmt.Sprintf("Features (kind=%s)", kindFilter)
		}
		return helpers.TextResult(helpers.FormatFeatureListMD(features, header)), nil
	}
}

// DeleteFeature removes a feature from a project.
func DeleteFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")

		if err := store.DeleteFeature(ctx, projectID, featureID); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Deleted feature %s from project %s", featureID, projectID)), nil
	}
}

// SearchFeatures performs a case-insensitive text search across feature titles
// and descriptions.
func SearchFeatures(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "query"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		query := strings.ToLower(helpers.GetString(req.Arguments, "query"))

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var matches []*types.FeatureData
		for _, f := range features {
			titleMatch := strings.Contains(strings.ToLower(f.Title), query)
			descMatch := strings.Contains(strings.ToLower(f.Description), query)
			if titleMatch || descMatch {
				matches = append(matches, f)
			}
		}

		kindFilter := helpers.GetString(req.Arguments, "kind")
		if kindFilter != "" {
			var filtered []*types.FeatureData
			for _, f := range matches {
				k := string(f.Kind)
				if k == "" {
					k = "feature"
				}
				if k == kindFilter {
					filtered = append(filtered, f)
				}
			}
			matches = filtered
		}

		if matches == nil {
			matches = []*types.FeatureData{}
		}

		header := fmt.Sprintf("Search results for %q", query)
		if kindFilter != "" {
			header = fmt.Sprintf("Search results for %q (kind=%s)", query, kindFilter)
		}
		return helpers.TextResult(helpers.FormatFeatureListMD(matches, header)), nil
	}
}
