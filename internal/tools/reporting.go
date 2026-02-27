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

func GetProgressSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func GetBlockedFeaturesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func GetReviewQueueSchema() *structpb.Struct {
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

// GetProgress returns completion percentage and feature counts by status.
func GetProgress(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		total := len(features)
		done := 0
		statusCounts := make(map[string]int)

		for _, f := range features {
			statusCounts[string(f.Status)]++
			if f.Status == types.StatusDone {
				done++
			}
		}

		var pctDone float64
		if total > 0 {
			pctDone = float64(done) / float64(total) * 100
		}

		md := fmt.Sprintf("## Progress: %s\n\n- **Total features:** %d\n- **Done:** %d\n- **Completion:** %.1f%%\n\n%s",
			projectID, total, done, pctDone, helpers.FormatStatusCountsMD(statusCounts, total))
		return helpers.TextResult(md), nil
	}
}

// GetBlockedFeatures returns features that are blocked by unfinished dependencies.
func GetBlockedFeatures(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Build a status map for quick lookup.
		statusMap := make(map[string]types.FeatureStatus)
		for _, f := range features {
			statusMap[f.ID] = f.Status
		}

		var b strings.Builder
		blockedCount := 0
		for _, f := range features {
			if len(f.DependsOn) == 0 {
				continue
			}
			var unblockers []string
			for _, depID := range f.DependsOn {
				depStatus, exists := statusMap[depID]
				if !exists || depStatus != types.StatusDone {
					unblockers = append(unblockers, depID)
				}
			}
			if len(unblockers) > 0 {
				fmt.Fprintf(&b, "- **%s** (%s) blocked by: %s\n", f.ID, f.Title, strings.Join(unblockers, ", "))
				blockedCount++
			}
		}

		if blockedCount == 0 {
			return helpers.TextResult("## Blocked Features\n\nNo blocked features found."), nil
		}

		md := fmt.Sprintf("## Blocked Features (%d)\n\n%s", blockedCount, b.String())
		return helpers.TextResult(md), nil
	}
}

// GetReviewQueue returns all features currently awaiting review.
func GetReviewQueue(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var inReview []*types.FeatureData
		for _, f := range features {
			if f.Status == types.StatusInReview {
				inReview = append(inReview, f)
			}
		}

		if inReview == nil {
			inReview = []*types.FeatureData{}
		}

		return helpers.TextResult(helpers.FormatFeatureListMD(inReview, "Review Queue")), nil
	}
}
