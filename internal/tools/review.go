package tools

import (
	"context"
	"fmt"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func SubmitReviewSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"status":     map[string]any{"type": "string", "description": "Review result", "enum": []any{"approved", "needs-edits"}},
			"comment":    map[string]any{"type": "string", "description": "Review comment"},
		},
		"required": []any{"project_id", "feature_id", "status"},
	})
	return s
}

func GetPendingReviewsSchema() *structpb.Struct {
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

// SubmitReview completes a review by either approving the feature (advancing to
// done) or requesting edits (setting to needs-edits). The agent MUST ask the
// user via AskUserQuestion before calling this tool.
func SubmitReview(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "status"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		reviewStatus := helpers.GetString(req.Arguments, "status")
		comment := helpers.GetString(req.Arguments, "comment")

		if err := helpers.ValidateOneOf(reviewStatus, "approved", "needs-edits"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if feat.Status != types.StatusInReview {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("feature must be in-review to submit review, currently %q", feat.Status)), nil
		}

		// SESSION LOCK CHECK: verify this session owns the feature.
		sessionID := req.GetSessionId()
		if sessionID != "" {
			if err := globaldb.CheckLock(projectID, featureID, sessionID); err != nil {
				return helpers.ErrorResult("session_lock", err.Error()), nil
			}
		}

		var newStatus types.FeatureStatus
		if reviewStatus == "approved" {
			newStatus = types.StatusDone
		} else {
			newStatus = types.StatusNeedsEdits
		}

		oldStatus := feat.Status
		feat.Status = newStatus
		feat.UpdatedAt = helpers.NowISO()

		if comment != "" {
			body += fmt.Sprintf("\n\n---\n**Review (%s)** (%s): %s\n", reviewStatus, helpers.NowISO(), comment)
		}

		// Release session lock when feature reaches done.
		if newStatus == types.StatusDone && sessionID != "" {
			globaldb.ReleaseLock(projectID, featureID)
		}

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		msg := fmt.Sprintf("Review submitted for **%s**: **%s** (%s -> %s)", featureID, reviewStatus, oldStatus, newStatus)
		msg += nextStepHint(featureID, newStatus)
		return helpers.TextResult(msg), nil
	}
}

// GetPendingReviews returns all features in the in-review status.
func GetPendingReviews(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var pending []*types.FeatureData
		for _, f := range features {
			if f.Status == types.StatusInReview {
				pending = append(pending, f)
			}
		}

		if pending == nil {
			pending = []*types.FeatureData{}
		}

		return helpers.TextResult(helpers.FormatFeatureListMD(pending, "Pending Reviews")), nil
	}
}

