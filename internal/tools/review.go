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

// ---------- Schemas ----------

func RequestReviewSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"evidence":   map[string]any{"type": "string", "description": "Self-review evidence with sections: ## Summary, ## Quality, ## Checklist. Call get_gate_requirements to see the full template."},
		},
		"required": []any{"project_id", "feature_id", "evidence"},
	})
	return s
}

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

// RequestReview transitions a feature to the in-review status. The feature
// must be in the documented state to be eligible for review. Requires
// self-review evidence from the agent. On success, instructs the agent to
// ask the human user for approval via AskUserQuestion before calling
// submit_review.
func RequestReview(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "evidence"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		evidence := helpers.GetString(req.Arguments, "evidence")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if !types.CanTransition(feat.Status, types.StatusInReview) {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("cannot request review from status %q", feat.Status)), nil
		}

		// Validate self-review evidence against the review gate.
		if err := types.ReviewGate.Validate(evidence); err != nil {
			msg := fmt.Sprintf("## Gate Blocked: %s\n\n**%s**\n\n%s",
				types.ReviewGate.Name, err.Error(), types.ReviewGate.Checklist)
			return helpers.ErrorResult("gate_blocked", msg), nil
		}

		oldStatus := feat.Status
		feat.Status = types.StatusInReview
		feat.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n\n---\n**Self-Review (%s -> in-review)** (%s):\n%s\n", oldStatus, helpers.NowISO(), evidence)

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		instruction := fmt.Sprintf(`Review requested for **%s** (%s -> in-review).

## Action Required

You MUST now present this review to the user for approval:

1. Use **AskUserQuestion** to show the user:
   - Feature: **%s** — %s
   - Self-review evidence provided above
   - Options: **"Approve"** / **"Needs Edits"**
2. Based on the user's response, call **submit_review** with:
   - status: "approved" or status: "needs-edits"
   - comment: the user's feedback (if any)

**Do NOT call submit_review without user approval.**`,
			featureID, oldStatus, featureID, feat.Title)

		return helpers.TextResult(instruction), nil
	}
}

// SubmitReview completes a review by either approving the feature (advancing to
// done) or requesting edits (setting to needs-edits).
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

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Review submitted for **%s**: **%s** (%s -> %s)", featureID, reviewStatus, oldStatus, newStatus)), nil
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
