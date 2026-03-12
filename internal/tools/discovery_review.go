package tools

import (
	"context"
	"encoding/json"
	"fmt"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreateDiscoveryReviewSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"cycle_id":   map[string]any{"type": "string", "description": "Discovery cycle this review belongs to (DISC-XXX)"},
			"title":      map[string]any{"type": "string", "description": "Review title (e.g. 'Week 1 Discovery Review')"},
		},
		"required": []any{"project_id", "cycle_id", "title"},
	})
	return s
}

func RecordReviewDecisionsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"review_id":   map[string]any{"type": "string", "description": "Discovery review ID (DREV-XXX)"},
			"surprises":   map[string]any{"type": "string", "description": "What surprised us about users?"},
			"wrong_about": map[string]any{"type": "string", "description": "What assumptions were wrong?"},
			"items":       map[string]any{"type": "string", "description": "JSON array of decisions: [{\"item_id\": \"HYPO-XXX\", \"item_type\": \"hypothesis\", \"decision\": \"continue|refine|pivot|stop\", \"rationale\": \"...\"}]"},
		},
		"required": []any{"project_id", "review_id", "surprises", "wrong_about", "items"},
	})
	return s
}

func GetDiscoveryReviewSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"review_id":  map[string]any{"type": "string", "description": "Discovery review ID (DREV-XXX)"},
		},
		"required": []any{"project_id", "review_id"},
	})
	return s
}

// ---------- Handlers ----------

func CreateDiscoveryReview(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "cycle_id", "title"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		cycleID := helpers.GetString(req.Arguments, "cycle_id")

		if _, _, _, err := store.ReadDiscoveryCycle(ctx, projectID, cycleID); err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("discovery cycle %q not found", cycleID)), nil
		}

		now := helpers.NowISO()
		reviewID := helpers.NewDiscoveryReviewID()
		review := &types.DiscoveryReviewData{
			ID:        reviewID,
			ProjectID: projectID,
			CycleID:   cycleID,
			Title:     helpers.GetString(req.Arguments, "title"),
			Version:   0,
			CreatedAt: now,
			UpdatedAt: now,
		}

		body := fmt.Sprintf("# %s\n\n**Cycle:** %s\n**Date:** %s\n", review.Title, cycleID, now)

		if _, err := store.WriteDiscoveryReview(ctx, projectID, reviewID, review, body, 0); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created discovery review **%s**: %s\n\n%s", reviewID, review.Title,
			helpers.FormatDiscoveryReviewMD(review))
		return helpers.TextResult(md), nil
	}
}

func RecordReviewDecisions(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "review_id", "surprises", "wrong_about", "items"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		reviewID := helpers.GetString(req.Arguments, "review_id")

		review, body, version, err := store.ReadDiscoveryReview(ctx, projectID, reviewID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("discovery review %q not found", reviewID)), nil
		}

		// Parse items JSON
		itemsJSON := helpers.GetString(req.Arguments, "items")
		var items []types.DiscoveryReviewItem
		if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
			return helpers.ErrorResult("validation_error", fmt.Sprintf("invalid items JSON: %v", err)), nil
		}

		// Validate decisions
		for _, item := range items {
			if err := helpers.ValidateOneOf(string(item.Decision), types.ValidReviewDecisions...); err != nil {
				return helpers.ErrorResult("validation_error", fmt.Sprintf("invalid decision for %s: %v", item.ItemID, err)), nil
			}
			if item.ItemType != "hypothesis" && item.ItemType != "experiment" {
				return helpers.ErrorResult("validation_error", fmt.Sprintf("item_type must be 'hypothesis' or 'experiment', got %q", item.ItemType)), nil
			}
		}

		review.Surprises = helpers.GetString(req.Arguments, "surprises")
		review.WrongAbout = helpers.GetString(req.Arguments, "wrong_about")
		review.Items = items
		review.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n---\n\n## Review Decisions (%s)\n\n**What surprised us:** %s\n\n**What we were wrong about:** %s\n\n",
			review.UpdatedAt, review.Surprises, review.WrongAbout)
		for _, item := range items {
			body += fmt.Sprintf("- **%s** (%s): %s — %s\n", item.ItemID, item.ItemType, item.Decision, item.Rationale)
		}

		if _, err := store.WriteDiscoveryReview(ctx, projectID, reviewID, review, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Recorded %d decisions in review **%s**\n\n%s",
			len(items), reviewID, helpers.FormatDiscoveryReviewMD(review))

		// Suggest follow-up actions based on decisions
		for _, item := range items {
			switch item.Decision {
			case types.DecisionPivot:
				if item.ItemType == "hypothesis" {
					md += fmt.Sprintf("\n- Action needed: `refine_hypothesis` for %s\n", item.ItemID)
				}
			case types.DecisionStop:
				if item.ItemType == "hypothesis" {
					md += fmt.Sprintf("\n- Action needed: `invalidate_hypothesis` for %s\n", item.ItemID)
				} else {
					md += fmt.Sprintf("\n- Action needed: `abandon_experiment` for %s\n", item.ItemID)
				}
			}
		}

		return helpers.TextResult(md), nil
	}
}

func GetDiscoveryReview(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "review_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		reviewID := helpers.GetString(req.Arguments, "review_id")

		review, body, _, err := store.ReadDiscoveryReview(ctx, projectID, reviewID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("discovery review %q not found", reviewID)), nil
		}

		md := helpers.FormatDiscoveryReviewMD(review)
		if body != "" {
			md += "\n---\n" + body
		}
		return helpers.TextResult(md), nil
	}
}
