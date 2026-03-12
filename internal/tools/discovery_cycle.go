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

func CreateDiscoveryCycleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"title":      map[string]any{"type": "string", "description": "Cycle title (e.g. 'Week 1: Contract Risk Discovery')"},
			"goal":       map[string]any{"type": "string", "description": "What we aim to learn this cycle"},
			"start_date": map[string]any{"type": "string", "description": "Cycle start date (ISO 8601)"},
			"end_date":   map[string]any{"type": "string", "description": "Cycle end date (ISO 8601)"},
		},
		"required": []any{"project_id", "title", "goal", "start_date", "end_date"},
	})
	return s
}

func GetDiscoveryCycleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"cycle_id":   map[string]any{"type": "string", "description": "Discovery cycle ID (DISC-XXX)"},
		},
		"required": []any{"project_id", "cycle_id"},
	})
	return s
}

func ListDiscoveryCyclesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (active/completed/cancelled)", "enum": []any{"active", "completed", "cancelled"}},
		},
		"required": []any{"project_id"},
	})
	return s
}

func UpdateDiscoveryCycleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"cycle_id":   map[string]any{"type": "string", "description": "Discovery cycle ID (DISC-XXX)"},
			"title":      map[string]any{"type": "string", "description": "New title"},
			"goal":       map[string]any{"type": "string", "description": "New goal"},
			"start_date": map[string]any{"type": "string", "description": "New start date"},
			"end_date":   map[string]any{"type": "string", "description": "New end date"},
		},
		"required": []any{"project_id", "cycle_id"},
	})
	return s
}

func CompleteDiscoveryCycleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"cycle_id":   map[string]any{"type": "string", "description": "Discovery cycle ID (DISC-XXX)"},
			"learnings":  map[string]any{"type": "string", "description": "Summary of what was learned during this cycle"},
			"decision":   map[string]any{"type": "string", "description": "Cycle decision", "enum": []any{"continue", "pivot", "stop"}},
		},
		"required": []any{"project_id", "cycle_id", "learnings", "decision"},
	})
	return s
}

func DeleteDiscoveryCycleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"cycle_id":   map[string]any{"type": "string", "description": "Discovery cycle ID (DISC-XXX)"},
		},
		"required": []any{"project_id", "cycle_id"},
	})
	return s
}

// ---------- Handlers ----------

func CreateDiscoveryCycle(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title", "goal", "start_date", "end_date"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		if _, _, err := store.ReadProject(ctx, projectID); err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		now := helpers.NowISO()
		cycleID := helpers.NewDiscoveryCycleID()
		cycle := &types.DiscoveryCycleData{
			ID:        cycleID,
			ProjectID: projectID,
			Title:     helpers.GetString(req.Arguments, "title"),
			Goal:      helpers.GetString(req.Arguments, "goal"),
			StartDate: helpers.GetString(req.Arguments, "start_date"),
			EndDate:   helpers.GetString(req.Arguments, "end_date"),
			Status:    types.CycleActive,
			Version:   0,
			CreatedAt: now,
			UpdatedAt: now,
		}

		body := fmt.Sprintf("# %s\n\n**Goal:** %s\n\n**Period:** %s to %s\n",
			cycle.Title, cycle.Goal, cycle.StartDate, cycle.EndDate)

		if _, err := store.WriteDiscoveryCycle(ctx, projectID, cycleID, cycle, body, 0); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created discovery cycle **%s**: %s\n\n%s", cycleID, cycle.Title,
			helpers.FormatDiscoveryCycleMD(cycle))
		return helpers.TextResult(md), nil
	}
}

func GetDiscoveryCycle(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "cycle_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		cycleID := helpers.GetString(req.Arguments, "cycle_id")

		cycle, body, _, err := store.ReadDiscoveryCycle(ctx, projectID, cycleID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("discovery cycle %q not found", cycleID)), nil
		}

		md := helpers.FormatDiscoveryCycleMD(cycle)
		if body != "" {
			md += "\n---\n" + body
		}
		return helpers.TextResult(md), nil
	}
}

func ListDiscoveryCycles(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")

		cycles, err := store.ListDiscoveryCycles(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if statusFilter != "" {
			var filtered []*types.DiscoveryCycleData
			for _, c := range cycles {
				if string(c.Status) == statusFilter {
					filtered = append(filtered, c)
				}
			}
			cycles = filtered
		}

		return helpers.TextResult(helpers.FormatDiscoveryCycleListMD(cycles, "Discovery Cycles")), nil
	}
}

func UpdateDiscoveryCycle(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "cycle_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		cycleID := helpers.GetString(req.Arguments, "cycle_id")

		cycle, body, version, err := store.ReadDiscoveryCycle(ctx, projectID, cycleID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("discovery cycle %q not found", cycleID)), nil
		}

		if t := helpers.GetString(req.Arguments, "title"); t != "" {
			cycle.Title = t
		}
		if g := helpers.GetString(req.Arguments, "goal"); g != "" {
			cycle.Goal = g
		}
		if sd := helpers.GetString(req.Arguments, "start_date"); sd != "" {
			cycle.StartDate = sd
		}
		if ed := helpers.GetString(req.Arguments, "end_date"); ed != "" {
			cycle.EndDate = ed
		}
		cycle.UpdatedAt = helpers.NowISO()

		if _, err := store.WriteDiscoveryCycle(ctx, projectID, cycleID, cycle, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Updated discovery cycle **%s**\n\n%s", cycleID,
			helpers.FormatDiscoveryCycleMD(cycle))), nil
	}
}

func CompleteDiscoveryCycle(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "cycle_id", "learnings", "decision"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		cycleID := helpers.GetString(req.Arguments, "cycle_id")
		decision := helpers.GetString(req.Arguments, "decision")

		if err := helpers.ValidateOneOf(decision, "continue", "pivot", "stop"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		cycle, body, version, err := store.ReadDiscoveryCycle(ctx, projectID, cycleID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("discovery cycle %q not found", cycleID)), nil
		}
		if cycle.Status != types.CycleActive {
			return helpers.ErrorResult("invalid_state", fmt.Sprintf("cycle is %s, not active", cycle.Status)), nil
		}

		cycle.Status = types.CycleCompleted
		cycle.Learnings = helpers.GetString(req.Arguments, "learnings")
		cycle.Decision = decision
		cycle.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n---\n\n## Cycle Completed (%s)\n\n**Decision:** %s\n\n**Learnings:**\n%s\n",
			cycle.UpdatedAt, decision, cycle.Learnings)

		if _, err := store.WriteDiscoveryCycle(ctx, projectID, cycleID, cycle, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Completed discovery cycle **%s** with decision: **%s**\n\n%s",
			cycleID, decision, helpers.FormatDiscoveryCycleMD(cycle))), nil
	}
}

func DeleteDiscoveryCycle(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "cycle_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		cycleID := helpers.GetString(req.Arguments, "cycle_id")

		if err := store.DeleteDiscoveryCycle(ctx, projectID, cycleID); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}
		return helpers.TextResult(fmt.Sprintf("Deleted discovery cycle **%s**", cycleID)), nil
	}
}
