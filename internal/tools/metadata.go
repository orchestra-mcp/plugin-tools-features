package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func AddLabelsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"labels":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Labels to add"},
		},
		"required": []any{"project_id", "feature_id", "labels"},
	})
	return s
}

func RemoveLabelsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"labels":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Labels to remove"},
		},
		"required": []any{"project_id", "feature_id", "labels"},
	})
	return s
}

func AssignFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"assignee":   map[string]any{"type": "string", "description": "Assignee name or ID"},
		},
		"required": []any{"project_id", "feature_id", "assignee"},
	})
	return s
}

func UnassignFeatureSchema() *structpb.Struct {
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

func SetEstimateSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"estimate":   map[string]any{"type": "string", "description": "Size estimate", "enum": []any{"S", "M", "L", "XL"}},
		},
		"required": []any{"project_id", "feature_id", "estimate"},
	})
	return s
}

func SaveNoteSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"note":       map[string]any{"type": "string", "description": "Note to append"},
		},
		"required": []any{"project_id", "feature_id", "note"},
	})
	return s
}

func ListNotesSchema() *structpb.Struct {
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

// ---------- Handlers ----------

// AddLabels adds labels to a feature.
func AddLabels(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		labels := helpers.GetStringSlice(req.Arguments, "labels")

		if len(labels) == 0 {
			return helpers.ErrorResult("validation_error", "labels must not be empty"), nil
		}

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Add labels that do not already exist.
		existing := make(map[string]bool)
		for _, l := range feat.Labels {
			existing[l] = true
		}
		for _, l := range labels {
			if !existing[l] {
				feat.Labels = append(feat.Labels, l)
			}
		}
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		labelsStr := strings.Join(feat.Labels, ", ")
		return helpers.TextResult(fmt.Sprintf("Added labels to **%s**. Current labels: %s", featureID, labelsStr)), nil
	}
}

// RemoveLabels removes labels from a feature.
func RemoveLabels(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		labels := helpers.GetStringSlice(req.Arguments, "labels")

		if len(labels) == 0 {
			return helpers.ErrorResult("validation_error", "labels must not be empty"), nil
		}

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		toRemove := make(map[string]bool)
		for _, l := range labels {
			toRemove[l] = true
		}

		var remaining []string
		for _, l := range feat.Labels {
			if !toRemove[l] {
				remaining = append(remaining, l)
			}
		}
		feat.Labels = remaining
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		labelsStr := "none"
		if len(feat.Labels) > 0 {
			labelsStr = strings.Join(feat.Labels, ", ")
		}
		return helpers.TextResult(fmt.Sprintf("Removed labels from **%s**. Current labels: %s", featureID, labelsStr)), nil
	}
}

// AssignFeature assigns a feature to a person.
func AssignFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "assignee"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		assignee := helpers.GetString(req.Arguments, "assignee")

		// If assignee is a person ID, validate it exists in the registry.
		if strings.HasPrefix(assignee, "PERS-") {
			_, _, _, err := store.ReadPerson(ctx, projectID, assignee)
			if err != nil {
				return helpers.ErrorResult("not_found",
					fmt.Sprintf("person %q not found in project %q", assignee, projectID)), nil
			}
		}

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		feat.Assignee = assignee
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Assigned **%s** to **%s**", featureID, assignee)), nil
	}
}

// UnassignFeature removes the assignee from a feature.
func UnassignFeature(store *storage.FeatureStorage) ToolHandler {
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

		feat.Assignee = ""
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Unassigned **%s**", featureID)), nil
	}
}

// SetEstimate sets the size estimate for a feature.
func SetEstimate(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "estimate"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		estimate := helpers.GetString(req.Arguments, "estimate")

		if err := helpers.ValidateOneOf(estimate, "S", "M", "L", "XL"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		feat.Estimate = estimate
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Set estimate for **%s** to **%s**", featureID, estimate)), nil
	}
}

// SaveNote appends a note to the feature's markdown body.
func SaveNote(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "note"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		note := helpers.GetString(req.Arguments, "note")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		body += fmt.Sprintf("\n\n## Note (%s)\n\n%s\n", helpers.NowISO(), note)
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Added note to %s", featureID)), nil
	}
}

// ListNotes returns the feature's markdown body as notes.
func ListNotes(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")

		_, body, _, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		md := fmt.Sprintf("## Notes for %s\n\n%s", featureID, body)
		return helpers.TextResult(md), nil
	}
}
