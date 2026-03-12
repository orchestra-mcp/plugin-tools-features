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

func CreateHypothesisSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"title":       map[string]any{"type": "string", "description": "Short hypothesis title"},
			"problem":     map[string]any{"type": "string", "description": "Problem statement (e.g. 'Startup founders struggle to understand legal risks in contracts')"},
			"target_user": map[string]any{"type": "string", "description": "Who experiences this problem (e.g. 'Early-stage startup founders signing vendor agreements')"},
			"assumption":  map[string]any{"type": "string", "description": "What we believe is true that needs validation"},
			"cycle_id":    map[string]any{"type": "string", "description": "Link to a discovery cycle (DISC-XXX)"},
			"labels":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional labels"},
		},
		"required": []any{"project_id", "title", "problem", "target_user", "assumption"},
	})
	return s
}

func GetHypothesisSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"hypothesis_id": map[string]any{"type": "string", "description": "Hypothesis ID (HYPO-XXX)"},
		},
		"required": []any{"project_id", "hypothesis_id"},
	})
	return s
}

func ListHypothesesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"status":     map[string]any{"type": "string", "description": "Filter by status", "enum": []any{"untested", "testing", "validated", "invalidated", "refined"}},
			"cycle_id":   map[string]any{"type": "string", "description": "Filter by cycle (DISC-XXX)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func UpdateHypothesisSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"hypothesis_id": map[string]any{"type": "string", "description": "Hypothesis ID (HYPO-XXX)"},
			"title":         map[string]any{"type": "string", "description": "New title"},
			"problem":       map[string]any{"type": "string", "description": "New problem statement"},
			"target_user":   map[string]any{"type": "string", "description": "New target user"},
			"assumption":    map[string]any{"type": "string", "description": "New assumption"},
		},
		"required": []any{"project_id", "hypothesis_id"},
	})
	return s
}

func ValidateHypothesisSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"hypothesis_id": map[string]any{"type": "string", "description": "Hypothesis ID (HYPO-XXX)"},
			"summary":       map[string]any{"type": "string", "description": "Validation summary — what evidence confirmed this hypothesis"},
		},
		"required": []any{"project_id", "hypothesis_id", "summary"},
	})
	return s
}

func InvalidateHypothesisSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"hypothesis_id": map[string]any{"type": "string", "description": "Hypothesis ID (HYPO-XXX)"},
			"reason":        map[string]any{"type": "string", "description": "Why this hypothesis was invalidated"},
		},
		"required": []any{"project_id", "hypothesis_id", "reason"},
	})
	return s
}

func RefineHypothesisSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"hypothesis_id": map[string]any{"type": "string", "description": "Original hypothesis ID to refine from (HYPO-XXX)"},
			"title":         map[string]any{"type": "string", "description": "New refined hypothesis title"},
			"problem":       map[string]any{"type": "string", "description": "Refined problem statement"},
			"target_user":   map[string]any{"type": "string", "description": "Refined target user"},
			"assumption":    map[string]any{"type": "string", "description": "Refined assumption"},
			"cycle_id":      map[string]any{"type": "string", "description": "Link to a discovery cycle (DISC-XXX)"},
		},
		"required": []any{"project_id", "hypothesis_id", "title", "problem", "target_user", "assumption"},
	})
	return s
}

// ---------- Handlers ----------

func CreateHypothesis(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title", "problem", "target_user", "assumption"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		if _, _, err := store.ReadProject(ctx, projectID); err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		now := helpers.NowISO()
		hypoID := helpers.NewHypothesisID()
		hypo := &types.HypothesisData{
			ID:         hypoID,
			ProjectID:  projectID,
			Title:      helpers.GetString(req.Arguments, "title"),
			Problem:    helpers.GetString(req.Arguments, "problem"),
			TargetUser: helpers.GetString(req.Arguments, "target_user"),
			Assumption: helpers.GetString(req.Arguments, "assumption"),
			Status:     types.HypoUntested,
			CycleID:    helpers.GetString(req.Arguments, "cycle_id"),
			Labels:     helpers.GetStringSlice(req.Arguments, "labels"),
			Version:    0,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		body := fmt.Sprintf("# %s\n\n**Problem:** %s\n\n**Target User:** %s\n\n**Assumption:** %s\n",
			hypo.Title, hypo.Problem, hypo.TargetUser, hypo.Assumption)

		if _, err := store.WriteHypothesis(ctx, projectID, hypoID, hypo, body, 0); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Link to cycle if provided
		if hypo.CycleID != "" {
			linkHypothesisToCycle(ctx, store, projectID, hypo.CycleID, hypoID)
		}

		md := fmt.Sprintf("Created hypothesis **%s**: %s\n\n%s", hypoID, hypo.Title,
			helpers.FormatHypothesisMD(hypo))
		return helpers.TextResult(md), nil
	}
}

func GetHypothesis(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "hypothesis_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		hypoID := helpers.GetString(req.Arguments, "hypothesis_id")

		hypo, body, _, err := store.ReadHypothesis(ctx, projectID, hypoID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("hypothesis %q not found", hypoID)), nil
		}

		md := helpers.FormatHypothesisMD(hypo)
		if body != "" {
			md += "\n---\n" + body
		}
		return helpers.TextResult(md), nil
	}
}

func ListHypotheses(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")
		cycleFilter := helpers.GetString(req.Arguments, "cycle_id")

		hypotheses, err := store.ListHypotheses(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var filtered []*types.HypothesisData
		for _, h := range hypotheses {
			if statusFilter != "" && string(h.Status) != statusFilter {
				continue
			}
			if cycleFilter != "" && h.CycleID != cycleFilter {
				continue
			}
			filtered = append(filtered, h)
		}

		return helpers.TextResult(helpers.FormatHypothesisListMD(filtered, "Hypotheses")), nil
	}
}

func UpdateHypothesis(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "hypothesis_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		hypoID := helpers.GetString(req.Arguments, "hypothesis_id")

		hypo, body, version, err := store.ReadHypothesis(ctx, projectID, hypoID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("hypothesis %q not found", hypoID)), nil
		}

		if t := helpers.GetString(req.Arguments, "title"); t != "" {
			hypo.Title = t
		}
		if p := helpers.GetString(req.Arguments, "problem"); p != "" {
			hypo.Problem = p
		}
		if tu := helpers.GetString(req.Arguments, "target_user"); tu != "" {
			hypo.TargetUser = tu
		}
		if a := helpers.GetString(req.Arguments, "assumption"); a != "" {
			hypo.Assumption = a
		}
		hypo.UpdatedAt = helpers.NowISO()

		if _, err := store.WriteHypothesis(ctx, projectID, hypoID, hypo, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Updated hypothesis **%s**\n\n%s", hypoID,
			helpers.FormatHypothesisMD(hypo))), nil
	}
}

func ValidateHypothesis(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "hypothesis_id", "summary"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		hypoID := helpers.GetString(req.Arguments, "hypothesis_id")
		summary := helpers.GetString(req.Arguments, "summary")

		hypo, body, version, err := store.ReadHypothesis(ctx, projectID, hypoID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("hypothesis %q not found", hypoID)), nil
		}

		hypo.Status = types.HypoValidated
		hypo.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n---\n\n## Validated (%s)\n\n%s\n", hypo.UpdatedAt, summary)

		if _, err := store.WriteHypothesis(ctx, projectID, hypoID, hypo, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Hypothesis **%s** validated.\n\n%s",
			hypoID, helpers.FormatHypothesisMD(hypo))), nil
	}
}

func InvalidateHypothesis(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "hypothesis_id", "reason"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		hypoID := helpers.GetString(req.Arguments, "hypothesis_id")
		reason := helpers.GetString(req.Arguments, "reason")

		hypo, body, version, err := store.ReadHypothesis(ctx, projectID, hypoID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("hypothesis %q not found", hypoID)), nil
		}

		hypo.Status = types.HypoInvalidated
		hypo.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n---\n\n## Invalidated (%s)\n\n%s\n", hypo.UpdatedAt, reason)

		if _, err := store.WriteHypothesis(ctx, projectID, hypoID, hypo, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Hypothesis **%s** invalidated.\n\n%s",
			hypoID, helpers.FormatHypothesisMD(hypo))), nil
	}
}

func RefineHypothesis(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "hypothesis_id", "title", "problem", "target_user", "assumption"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		origID := helpers.GetString(req.Arguments, "hypothesis_id")

		// Mark original as refined
		orig, origBody, origVersion, err := store.ReadHypothesis(ctx, projectID, origID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("hypothesis %q not found", origID)), nil
		}
		orig.Status = types.HypoRefined
		orig.UpdatedAt = helpers.NowISO()
		origBody += fmt.Sprintf("\n---\n\n## Refined (%s)\n\nRefined into a new hypothesis.\n", orig.UpdatedAt)
		if _, err := store.WriteHypothesis(ctx, projectID, origID, orig, origBody, origVersion); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Create new refined hypothesis
		now := helpers.NowISO()
		newID := helpers.NewHypothesisID()
		cycleID := helpers.GetString(req.Arguments, "cycle_id")
		if cycleID == "" {
			cycleID = orig.CycleID
		}

		newHypo := &types.HypothesisData{
			ID:          newID,
			ProjectID:   projectID,
			Title:       helpers.GetString(req.Arguments, "title"),
			Problem:     helpers.GetString(req.Arguments, "problem"),
			TargetUser:  helpers.GetString(req.Arguments, "target_user"),
			Assumption:  helpers.GetString(req.Arguments, "assumption"),
			Status:      types.HypoUntested,
			CycleID:     cycleID,
			RefinedFrom: origID,
			Labels:      orig.Labels,
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n**Refined from:** %s\n\n**Problem:** %s\n\n**Target User:** %s\n\n**Assumption:** %s\n",
			newHypo.Title, origID, newHypo.Problem, newHypo.TargetUser, newHypo.Assumption)

		if _, err := store.WriteHypothesis(ctx, projectID, newID, newHypo, body, 0); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if newHypo.CycleID != "" {
			linkHypothesisToCycle(ctx, store, projectID, newHypo.CycleID, newID)
		}

		md := fmt.Sprintf("Refined **%s** → **%s**: %s\n\n%s", origID, newID, newHypo.Title,
			helpers.FormatHypothesisMD(newHypo))
		return helpers.TextResult(md), nil
	}
}

// linkHypothesisToCycle adds a hypothesis ID to a cycle's Hypotheses list (best-effort).
func linkHypothesisToCycle(ctx context.Context, store *storage.FeatureStorage, projectID, cycleID, hypoID string) {
	cycle, body, version, err := store.ReadDiscoveryCycle(ctx, projectID, cycleID)
	if err != nil {
		return
	}
	for _, h := range cycle.Hypotheses {
		if h == hypoID {
			return
		}
	}
	cycle.Hypotheses = append(cycle.Hypotheses, hypoID)
	cycle.UpdatedAt = helpers.NowISO()
	_, _ = store.WriteDiscoveryCycle(ctx, projectID, cycleID, cycle, body, version)
}

// linkExperimentToCycle adds an experiment ID to a cycle's Experiments list (best-effort).
func linkExperimentToCycle(ctx context.Context, store *storage.FeatureStorage, projectID, cycleID, exprID string) {
	cycle, body, version, err := store.ReadDiscoveryCycle(ctx, projectID, cycleID)
	if err != nil {
		return
	}
	for _, e := range cycle.Experiments {
		if e == exprID {
			return
		}
	}
	cycle.Experiments = append(cycle.Experiments, exprID)
	cycle.UpdatedAt = helpers.NowISO()
	_, _ = store.WriteDiscoveryCycle(ctx, projectID, cycleID, cycle, body, version)
}

// linkExperimentToHypothesis adds an experiment ID to a hypothesis's Experiments list (best-effort).
func linkExperimentToHypothesis(ctx context.Context, store *storage.FeatureStorage, projectID, hypoID, exprID string) {
	hypo, body, version, err := store.ReadHypothesis(ctx, projectID, hypoID)
	if err != nil {
		return
	}
	for _, e := range hypo.Experiments {
		if e == exprID {
			return
		}
	}
	hypo.Experiments = append(hypo.Experiments, exprID)
	if hypo.Status == types.HypoUntested {
		hypo.Status = types.HypoTesting
	}
	hypo.UpdatedAt = helpers.NowISO()
	_, _ = store.WriteHypothesis(ctx, projectID, hypoID, hypo, body, version)
}

// suppress unused import warnings
var _ = strings.Join
