package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func AddDependencySchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"feature_id":    map[string]any{"type": "string", "description": "Feature that depends on another"},
			"depends_on_id": map[string]any{"type": "string", "description": "Feature that is depended upon"},
		},
		"required": []any{"project_id", "feature_id", "depends_on_id"},
	})
	return s
}

func RemoveDependencySchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"feature_id":    map[string]any{"type": "string", "description": "Feature to remove dependency from"},
			"depends_on_id": map[string]any{"type": "string", "description": "Feature to remove from dependencies"},
		},
		"required": []any{"project_id", "feature_id", "depends_on_id"},
	})
	return s
}

func GetDependencyGraphSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

// ---------- Handlers ----------

// AddDependency adds a dependency relationship: feature_id depends on depends_on_id.
// It also updates the blocks list on the target feature.
func AddDependency(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "depends_on_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		dependsOnID := helpers.GetString(req.Arguments, "depends_on_id")

		if featureID == dependsOnID {
			return helpers.ErrorResult("validation_error", "a feature cannot depend on itself"), nil
		}

		// Read the dependent feature.
		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("feature %q not found", featureID)), nil
		}

		// Verify the target feature exists.
		target, targetBody, targetVersion, err := store.ReadFeature(ctx, projectID, dependsOnID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("dependency target %q not found", dependsOnID)), nil
		}

		// Check for duplicate.
		for _, d := range feat.DependsOn {
			if d == dependsOnID {
				return helpers.TextResult(fmt.Sprintf("Dependency already exists: %s depends on %s", featureID, dependsOnID)), nil
			}
		}

		// Add dependency.
		feat.DependsOn = append(feat.DependsOn, dependsOnID)
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Add to blocks list of target.
		target.Blocks = append(target.Blocks, featureID)
		target.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, dependsOnID, target, targetBody, targetVersion)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Added dependency: **%s** depends on **%s**", featureID, dependsOnID)), nil
	}
}

// RemoveDependency removes a dependency relationship.
func RemoveDependency(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "depends_on_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		dependsOnID := helpers.GetString(req.Arguments, "depends_on_id")

		// Update the dependent feature.
		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		feat.DependsOn = removeFromSlice(feat.DependsOn, dependsOnID)
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Update the target feature's blocks list.
		target, targetBody, targetVersion, err := store.ReadFeature(ctx, projectID, dependsOnID)
		if err == nil {
			target.Blocks = removeFromSlice(target.Blocks, featureID)
			target.UpdatedAt = helpers.NowISO()
			_, _ = store.WriteFeature(ctx, projectID, dependsOnID, target, targetBody, targetVersion)
		}

		return helpers.TextResult(fmt.Sprintf("Removed dependency: **%s** no longer depends on **%s**", featureID, dependsOnID)), nil
	}
}

// GetDependencyGraph returns all dependency relationships in a project.
func GetDependencyGraph(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Dependency Graph (%d features)\n\n", len(features))

		hasEdges := false
		for _, f := range features {
			for _, dep := range f.DependsOn {
				fmt.Fprintf(&b, "- **%s** depends on **%s**\n", f.ID, dep)
				hasEdges = true
			}
		}
		if !hasEdges {
			b.WriteString("No dependencies found.\n")
		}

		return helpers.TextResult(b.String()), nil
	}
}

// removeFromSlice removes the first occurrence of val from the slice.
func removeFromSlice(slice []string, val string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != val {
			result = append(result, s)
		}
	}
	return result
}
