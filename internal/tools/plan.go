package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreatePlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"title":       map[string]any{"type": "string", "description": "Plan title"},
			"description": map[string]any{"type": "string", "description": "Plan description"},
		},
		"required": []any{"project_id", "title"},
	})
	return s
}

func GetPlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"plan_id":    map[string]any{"type": "string", "description": "Plan ID (e.g., PLAN-ABC)"},
		},
		"required": []any{"project_id", "plan_id"},
	})
	return s
}

func ListPlansSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)"},
			"limit":      map[string]any{"type": "number", "description": "Max results (default 50, max 200)"},
			"offset":     map[string]any{"type": "number", "description": "Skip first N results (default 0)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func UpdatePlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"plan_id":     map[string]any{"type": "string", "description": "Plan ID"},
			"title":       map[string]any{"type": "string", "description": "New title"},
			"description": map[string]any{"type": "string", "description": "New description"},
		},
		"required": []any{"project_id", "plan_id"},
	})
	return s
}

func ApprovePlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"plan_id":    map[string]any{"type": "string", "description": "Plan ID"},
		},
		"required": []any{"project_id", "plan_id"},
	})
	return s
}

func BreakdownPlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"plan_id":    map[string]any{"type": "string", "description": "Plan ID"},
			"features": map[string]any{
				"type":        "array",
				"description": "Array of feature objects with fields: title, description, priority, kind, estimate, depends_on",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":       map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"priority":    map[string]any{"type": "string"},
						"kind":        map[string]any{"type": "string"},
						"estimate":    map[string]any{"type": "string"},
						"depends_on":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required": []any{"title"},
				},
			},
		},
		"required": []any{"project_id", "plan_id", "features"},
	})
	return s
}

func CompletePlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"plan_id":    map[string]any{"type": "string", "description": "Plan ID"},
		},
		"required": []any{"project_id", "plan_id"},
	})
	return s
}

// ---------- Handlers ----------

// CreatePlan creates a new plan in a project.
func CreatePlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		title := helpers.GetString(req.Arguments, "title")
		description := helpers.GetString(req.Arguments, "description")

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		planID := helpers.NewPlanID()
		now := helpers.NowISO()

		plan := &types.PlanData{
			ID:          planID,
			ProjectID:   projectID,
			Title:       title,
			Description: description,
			Status:      types.PlanDraft,
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n%s\n", title, description)

		_, err = store.WritePlan(ctx, projectID, planID, plan, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created **%s**: %s\n\n%s", planID, title, helpers.FormatPlanMD(plan))
		ui := helpers.PlanDetailUI(plan,
			helpers.UIAction{Label: "Approve", Tool: "approve_plan", Params: map[string]any{"project_id": projectID, "plan_id": planID}, Kind: "primary"},
		)
		return helpers.RichResult(md, ui), nil
	}
}

// GetPlan returns a plan's data and body.
func GetPlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "plan_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		planID := helpers.GetString(req.Arguments, "plan_id")

		plan, body, _, err := store.ReadPlan(ctx, projectID, planID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		md := helpers.FormatPlanMD(plan) + "\n---\n\n" + body
		ui := helpers.PlanDetailUI(plan)
		return helpers.RichResult(md, ui), nil
	}
}

// ListPlans returns all plans in a project, optionally filtered by status.
func ListPlans(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")

		plans, err := store.ListPlans(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if statusFilter != "" {
			var filtered []*types.PlanData
			for _, p := range plans {
				if string(p.Status) == statusFilter {
					filtered = append(filtered, p)
				}
			}
			plans = filtered
		}

		if plans == nil {
			plans = []*types.PlanData{}
		}

		total := len(plans)
		pg := helpers.ParsePagination(req.Arguments)
		plans = helpers.PaginateSlice(plans, pg)

		header := "Plans"
		if statusFilter != "" {
			header = fmt.Sprintf("Plans (%s)", statusFilter)
		}
		md := helpers.FormatPlanListMD(plans, header)
		md += fmt.Sprintf("\n*Showing %d-%d of %d total*\n", pg.Offset+1, pg.Offset+len(plans), total)
		ui := helpers.PlanListUI(plans, pg, total)
		return helpers.RichResult(md, ui), nil
	}
}

// UpdatePlan updates mutable fields of a plan.
func UpdatePlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "plan_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		planID := helpers.GetString(req.Arguments, "plan_id")

		plan, body, version, err := store.ReadPlan(ctx, projectID, planID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if t := helpers.GetString(req.Arguments, "title"); t != "" {
			plan.Title = t
		}
		if d := helpers.GetString(req.Arguments, "description"); d != "" {
			plan.Description = d
			body = fmt.Sprintf("# %s\n\n%s\n", plan.Title, d)
		}

		plan.UpdatedAt = helpers.NowISO()

		_, err = store.WritePlan(ctx, projectID, planID, plan, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Updated **%s**\n\n%s", planID, helpers.FormatPlanMD(plan))
		ui := helpers.PlanDetailUI(plan)
		return helpers.RichResult(md, ui), nil
	}
}

// ApprovePlan transitions a plan from draft to approved.
func ApprovePlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "plan_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		planID := helpers.GetString(req.Arguments, "plan_id")

		plan, body, version, err := store.ReadPlan(ctx, projectID, planID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if plan.Status != types.PlanDraft {
			return helpers.ErrorResult("invalid_state",
				fmt.Sprintf("plan %s is %q, must be %q to approve", planID, plan.Status, types.PlanDraft)), nil
		}

		plan.Status = types.PlanApproved
		plan.UpdatedAt = helpers.NowISO()

		_, err = store.WritePlan(ctx, projectID, planID, plan, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Approved **%s** (draft → approved)", planID)), nil
	}
}

// breakdownFeature is a local struct for parsing breakdown input.
type breakdownFeature struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Kind        string   `json:"kind"`
	Estimate    string   `json:"estimate"`
	DependsOn   []string `json:"depends_on"`
}

// BreakdownPlan creates features from an approved plan and links them.
func BreakdownPlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "plan_id", "features"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		planID := helpers.GetString(req.Arguments, "plan_id")

		// Read and validate plan.
		plan, body, version, err := store.ReadPlan(ctx, projectID, planID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if plan.Status != types.PlanApproved && plan.Status != types.PlanInProgress {
			return helpers.ErrorResult("invalid_state",
				fmt.Sprintf("plan %s is %q, must be %q or %q to break down", planID, plan.Status, types.PlanApproved, types.PlanInProgress)), nil
		}

		// Parse features — accept either a native JSON array or a JSON-encoded string.
		var defs []breakdownFeature
		if raw, ok := req.Arguments.Fields["features"]; ok {
			switch v := raw.Kind.(type) {
			case *structpb.Value_StringValue:
				// Legacy: caller passed a JSON-encoded string
				if err := json.Unmarshal([]byte(v.StringValue), &defs); err != nil {
					return helpers.ErrorResult("validation_error", fmt.Sprintf("invalid features JSON: %s", err.Error())), nil
				}
			case *structpb.Value_ListValue:
				// Preferred: caller passed a native JSON array
				b, _ := json.Marshal(v.ListValue.AsSlice())
				if err := json.Unmarshal(b, &defs); err != nil {
					return helpers.ErrorResult("validation_error", fmt.Sprintf("invalid features array: %s", err.Error())), nil
				}
			default:
				return helpers.ErrorResult("validation_error", "features must be an array of feature objects"), nil
			}
		}

		if len(defs) == 0 {
			return helpers.ErrorResult("validation_error", "features array must not be empty"), nil
		}

		now := helpers.NowISO()
		createdIDs := make([]string, len(defs))

		// Create each feature. Skip creation if the plan already has this slot filled
		// (idempotent retry: plan was partially broken down before).
		existingIDs := plan.Features
		for i, def := range defs {
			// Reuse existing ID if the plan already has a feature at this position.
			if i < len(existingIDs) {
				createdIDs[i] = existingIDs[i]
				continue
			}

			featureID := helpers.NewFeatureID()
			createdIDs[i] = featureID

			priority := def.Priority
			if priority == "" {
				priority = "P2"
			}

			kind := types.FeatureKind(def.Kind)
			if def.Kind == "" {
				kind = types.KindFeature
			}

			feat := &types.FeatureData{
				ID:          featureID,
				ProjectID:   projectID,
				Title:       def.Title,
				Description: def.Description,
				Status:      types.StatusTodo,
				Priority:    priority,
				Kind:        kind,
				Estimate:    def.Estimate,
				Labels:      []string{fmt.Sprintf("plan:%s", planID)},
				Version:     0,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			featBody := fmt.Sprintf("# %s\n\n%s\n", def.Title, def.Description)

			_, err := store.WriteFeature(ctx, projectID, featureID, feat, featBody, 0)
			if err != nil {
				return helpers.ErrorResult("storage_error",
					fmt.Sprintf("failed to create feature %d (%s): %s", i, def.Title, err.Error())), nil
			}
		}

		// Resolve depends_on references (indexes → feature IDs).
		for i, def := range defs {
			if len(def.DependsOn) == 0 {
				continue
			}

			var depIDs []string
			for _, ref := range def.DependsOn {
				idx, err := strconv.Atoi(ref)
				if err != nil || idx < 0 || idx >= len(createdIDs) {
					continue // skip invalid references
				}
				depIDs = append(depIDs, createdIDs[idx])
			}

			if len(depIDs) == 0 {
				continue
			}

			feat, featBody, featVersion, err := store.ReadFeature(ctx, projectID, createdIDs[i])
			if err != nil {
				continue
			}

			feat.DependsOn = depIDs
			feat.UpdatedAt = helpers.NowISO()

			_, err = store.WriteFeature(ctx, projectID, createdIDs[i], feat, featBody, featVersion)
			if err != nil {
				continue
			}
		}

		// Update plan with linked features and status.
		plan.Features = createdIDs
		plan.Status = types.PlanInProgress
		plan.UpdatedAt = helpers.NowISO()

		_, err = store.WritePlan(ctx, projectID, planID, plan, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Build result.
		var b strings.Builder
		fmt.Fprintf(&b, "Broke down **%s** into %d features (approved → in-progress)\n\n", planID, len(createdIDs))
		for i, id := range createdIDs {
			fmt.Fprintf(&b, "- **%s**: %s\n", id, defs[i].Title)
		}

		return helpers.TextResult(b.String()), nil
	}
}

// CompletePlan transitions a plan to completed after verifying all features are done.
func CompletePlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "plan_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		planID := helpers.GetString(req.Arguments, "plan_id")

		plan, body, version, err := store.ReadPlan(ctx, projectID, planID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if plan.Status != types.PlanInProgress {
			return helpers.ErrorResult("invalid_state",
				fmt.Sprintf("plan %s is %q, must be %q to complete", planID, plan.Status, types.PlanInProgress)), nil
		}

		// Verify all linked features are done.
		var notDone []string
		for _, featID := range plan.Features {
			feat, _, _, err := store.ReadFeature(ctx, projectID, featID)
			if err != nil {
				notDone = append(notDone, fmt.Sprintf("%s (not found)", featID))
				continue
			}
			if feat.Status != types.StatusDone {
				notDone = append(notDone, fmt.Sprintf("%s (%s)", featID, feat.Status))
			}
		}

		if len(notDone) > 0 {
			return helpers.ErrorResult("incomplete",
				fmt.Sprintf("cannot complete plan — %d feature(s) not done: %s",
					len(notDone), strings.Join(notDone, ", "))), nil
		}

		plan.Status = types.PlanCompleted
		plan.UpdatedAt = helpers.NowISO()

		_, err = store.WritePlan(ctx, projectID, planID, plan, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(
			fmt.Sprintf("Completed **%s** — all %d features done", planID, len(plan.Features))), nil
	}
}

// DeletePlanSchema returns the JSON Schema for the delete_plan tool.
func DeletePlanSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"plan_id":    map[string]any{"type": "string", "description": "Plan ID (e.g., PLAN-ABC)"},
		},
		"required": []any{"project_id", "plan_id"},
	})
	return s
}

// DeletePlan removes a plan from a project.
func DeletePlan(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "plan_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		planID := helpers.GetString(req.Arguments, "plan_id")

		if err := store.DeletePlan(ctx, projectID, planID); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Deleted plan: %s", planID)), nil
	}
}
