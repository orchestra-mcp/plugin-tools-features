// Package tools contains all tool handler implementations for the
// tools.features plugin. Each function returns a plugin.ToolHandler closure
// that captures the FeatureStorage for data access.
package tools

import (
	"context"
	"fmt"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ToolHandler is an alias for readability.
type ToolHandler = func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error)

// ---------- Schemas ----------

func CreateProjectSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "Project name"},
			"description": map[string]any{"type": "string", "description": "Project description"},
		},
		"required": []any{"name"},
	})
	return s
}

func ListProjectsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	})
	return s
}

func DeleteProjectSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func GetProjectStatusSchema() *structpb.Struct {
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

// CreateProject creates a new project with a slug derived from the name.
func CreateProject(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "name"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		name := helpers.GetString(req.Arguments, "name")
		description := helpers.GetString(req.Arguments, "description")
		slug := helpers.Slugify(name)
		now := helpers.NowISO()

		proj := &types.ProjectData{
			ID:          slug,
			Name:        name,
			Slug:        slug,
			Description: description,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		_, err := store.WriteProject(ctx, proj, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Created project: %s (slug: %s)", name, slug)), nil
	}
}

// ListProjects returns all projects in storage.
func ListProjects(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		projects, err := store.ListProjects(ctx)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(helpers.FormatProjectListMD(projects)), nil
	}
}

// DeleteProject removes a project and all its features.
func DeleteProject(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		if err := store.DeleteProject(ctx, projectID); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Deleted project: %s", projectID)), nil
	}
}

// GetProjectStatus returns a project along with feature counts grouped by status.
func GetProjectStatus(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		proj, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		statusCounts := make(map[string]int)
		for _, f := range features {
			statusCounts[string(f.Status)]++
		}

		md := helpers.FormatProjectMD(proj) + "\n" + helpers.FormatStatusCountsMD(statusCounts, len(features))
		return helpers.TextResult(md), nil
	}
}
