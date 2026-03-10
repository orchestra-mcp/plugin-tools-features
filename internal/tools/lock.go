package tools

import (
	"context"
	"fmt"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func UnlockFeatureSchema() *structpb.Struct {
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

// UnlockFeature force-releases a session lock on a feature. This is an admin
// recovery tool — it does NOT check the calling session. Use when a session
// crashed or timed out and left a stale lock.
func UnlockFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")

		// Verify the feature exists.
		_, _, _, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Check if there's actually a lock to release.
		info, err := globaldb.GetLockInfo(projectID, featureID)
		if err != nil || info == nil {
			return helpers.TextResult(fmt.Sprintf("Feature **%s** is not locked.", featureID)), nil
		}

		globaldb.ReleaseLock(projectID, featureID)

		return helpers.TextResult(fmt.Sprintf(
			"Unlocked **%s** (was locked by session `%s` since %s).",
			featureID, info.SessionID, info.LockedAt)), nil
	}
}
