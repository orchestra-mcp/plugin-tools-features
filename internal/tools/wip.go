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

// WIP limits are stored in a per-project config file at:
// .projects/{slug}/wip.json

// ---------- Schemas ----------

func SetWIPLimitsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":      map[string]any{"type": "string", "description": "Project slug"},
			"max_in_progress": map[string]any{"type": "number", "description": "Maximum features allowed in-progress"},
		},
		"required": []any{"project_id", "max_in_progress"},
	})
	return s
}

func GetWIPLimitsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func CheckWIPLimitSchema() *structpb.Struct {
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

// SetWIPLimits stores the WIP limit configuration for a project. The limit is
// stored as metadata on a dedicated wip.json file within the project directory.
func SetWIPLimits(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		maxInProgress := helpers.GetInt(req.Arguments, "max_in_progress")

		if maxInProgress <= 0 {
			return helpers.ErrorResult("validation_error", "max_in_progress must be a positive integer"), nil
		}

		wipStore := NewWIPStore(store)
		err := wipStore.Set(ctx, projectID, maxInProgress)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Set WIP limit to **%d** for project **%s**", maxInProgress, projectID)), nil
	}
}

// GetWIPLimits returns the current WIP limits for a project.
func GetWIPLimits(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		wipStore := NewWIPStore(store)
		limit, err := wipStore.Get(ctx, projectID)
		if err != nil {
			return helpers.TextResult(fmt.Sprintf("No WIP limits configured for project **%s**", projectID)), nil
		}

		return helpers.TextResult(fmt.Sprintf("WIP limit for **%s**: **%d** max in-progress", projectID, limit)), nil
	}
}

// CheckWIPLimit checks whether the WIP limit would be exceeded.
func CheckWIPLimit(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		wipStore := NewWIPStore(store)
		limit, err := wipStore.Get(ctx, projectID)
		if err != nil {
			return helpers.TextResult(fmt.Sprintf("No WIP limits configured for project **%s** -- all clear", projectID)), nil
		}

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		current := 0
		for _, f := range features {
			if f.Status == types.StatusInProgress {
				current++
			}
		}

		withinLimit := current < limit
		status := "within limit"
		if !withinLimit {
			status = "LIMIT REACHED"
		}

		md := fmt.Sprintf("**WIP Check: %s**\n\n- **In-progress:** %d / %d\n- **Status:** %s", projectID, current, limit, status)
		return helpers.TextResult(md), nil
	}
}

// ---------- WIPStore ----------

// WIPStore handles reading and writing WIP limit configuration. It stores
// the limit as a feature-like file at .projects/{slug}/wip.json, using the
// same storage protocol as features and projects.
type WIPStore struct {
	store *storage.FeatureStorage
}

// NewWIPStore creates a new WIPStore.
func NewWIPStore(store *storage.FeatureStorage) *WIPStore {
	return &WIPStore{store: store}
}

// wipPath returns the storage path for WIP config.
func wipPath(projectSlug string) string {
	return projectSlug + "/wip.json"
}

// Set stores the WIP limit for a project.
func (w *WIPStore) Set(ctx context.Context, projectSlug string, maxInProgress int) error {
	// Try to read existing to get version for CAS.
	meta, _ := structpb.NewStruct(map[string]any{
		"max_in_progress": float64(maxInProgress),
	})

	// Try read first to get version.
	existingResp, err := w.store.ReadWIPConfig(ctx, projectSlug)
	var expectedVersion int64
	if err == nil {
		expectedVersion = existingResp
	}

	return w.store.WriteWIPConfig(ctx, projectSlug, meta, expectedVersion)
}

// Get reads the WIP limit for a project.
func (w *WIPStore) Get(ctx context.Context, projectSlug string) (int, error) {
	meta, err := w.store.ReadWIPMetadata(ctx, projectSlug)
	if err != nil {
		return 0, err
	}
	limit := int(meta.Fields["max_in_progress"].GetNumberValue())
	if limit <= 0 {
		return 0, fmt.Errorf("no WIP limit configured")
	}
	return limit, nil
}
