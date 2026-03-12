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

func SetProjectModeSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"mode":       map[string]any{"type": "string", "description": "Operating mode", "enum": []any{"discovery", "outcome", "scale"}},
		},
		"required": []any{"project_id", "mode"},
	})
	return s
}

func GetProjectModeSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func CheckTransitionSignalsSchema() *structpb.Struct {
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

func SetProjectMode(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "mode"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		mode := helpers.GetString(req.Arguments, "mode")

		if err := helpers.ValidateOneOf(mode, types.ValidModes...); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		proj, version, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		oldMode := string(proj.Mode)
		if oldMode == "" {
			oldMode = "outcome"
		}

		proj.Mode = types.ProjectMode(mode)
		proj.UpdatedAt = helpers.NowISO()

		if _, err := store.WriteProject(ctx, proj, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Project **%s** mode changed: **%s** → **%s**",
			projectID, oldMode, mode)), nil
	}
}

func GetProjectMode(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")

		proj, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		mode := string(proj.Mode)
		if mode == "" {
			mode = "outcome"
		}

		description := ""
		switch types.ProjectMode(mode) {
		case types.ModeDiscovery:
			description = "Discovery Mode — validate problem hypotheses through experiments before engineering"
		case types.ModeOutcome:
			description = "Outcome Mode — build validated features through gated delivery cycles"
		case types.ModeScale:
			description = "Scale Mode — optimize and grow with established product-market fit"
		}

		return helpers.TextResult(fmt.Sprintf("## Project Mode: %s\n\n%s", mode, description)), nil
	}
}

func CheckTransitionSignals(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")

		// Get project mode
		proj, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}
		currentMode := string(proj.Mode)
		if currentMode == "" {
			currentMode = "outcome"
		}

		// Gather hypothesis stats
		hypotheses, _ := store.ListHypotheses(ctx, projectID)
		hypoCounts := map[string]int{}
		for _, h := range hypotheses {
			hypoCounts[string(h.Status)]++
		}

		// Gather experiment stats
		experiments, _ := store.ListExperiments(ctx, projectID)
		exprCounts := map[string]int{}
		signalCounts := map[string]int{}
		killCount := 0
		for _, e := range experiments {
			exprCounts[string(e.Status)]++
			if e.KillTriggered {
				killCount++
			}
			for _, s := range e.Signals {
				signalCounts[string(s.Type)]++
			}
		}

		// Build assessment
		var md string
		md += fmt.Sprintf("## Transition Signal Analysis\n\n")
		md += fmt.Sprintf("**Current Mode:** %s\n\n", currentMode)

		// Hypothesis summary
		totalHypos := len(hypotheses)
		validated := hypoCounts["validated"]
		invalidated := hypoCounts["invalidated"]
		md += fmt.Sprintf("### Hypotheses (%d total)\n", totalHypos)
		md += fmt.Sprintf("| Status | Count |\n|--------|-------|\n")
		for _, status := range types.ValidHypothesisStatuses {
			if c := hypoCounts[status]; c > 0 {
				md += fmt.Sprintf("| %s | %d |\n", status, c)
			}
		}

		// Experiment summary
		totalExprs := len(experiments)
		completedExprs := exprCounts["completed"]
		md += fmt.Sprintf("\n### Experiments (%d total)\n", totalExprs)
		md += fmt.Sprintf("| Status | Count |\n|--------|-------|\n")
		for _, status := range types.ValidExperimentStatuses {
			if c := exprCounts[status]; c > 0 {
				md += fmt.Sprintf("| %s | %d |\n", status, c)
			}
		}

		// Signal summary
		totalSignals := signalCounts["user"] + signalCounts["behavior"] + signalCounts["market"]
		md += fmt.Sprintf("\n### Signals (%d total)\n", totalSignals)
		md += fmt.Sprintf("- User signals: %d\n", signalCounts["user"])
		md += fmt.Sprintf("- Behavior signals: %d\n", signalCounts["behavior"])
		md += fmt.Sprintf("- Market signals: %d\n", signalCounts["market"])
		if killCount > 0 {
			md += fmt.Sprintf("- Kill conditions triggered: %d\n", killCount)
		}

		// Transition recommendation (discovery → outcome)
		if currentMode == "discovery" {
			md += "\n### Transition Readiness: Discovery → Outcome\n\n"

			ready := true
			checks := []struct {
				name string
				pass bool
				note string
			}{
				{"Validated hypotheses", validated >= 1, fmt.Sprintf("%d validated (need ≥1)", validated)},
				{"Completed experiments", completedExprs >= 1, fmt.Sprintf("%d completed (need ≥1)", completedExprs)},
				{"Behavior signals", signalCounts["behavior"] >= 1, fmt.Sprintf("%d behavior signals (need ≥1)", signalCounts["behavior"])},
				{"No active kill conditions", killCount == 0 || completedExprs > killCount, fmt.Sprintf("%d kills vs %d completed", killCount, completedExprs)},
			}

			for _, c := range checks {
				icon := "pass"
				if !c.pass {
					icon = "fail"
					ready = false
				}
				md += fmt.Sprintf("- [%s] %s — %s\n", icon, c.name, c.note)
			}

			if ready {
				md += "\n**Recommendation:** Ready to transition to Outcome Mode. Use `set_project_mode` to switch.\n"
			} else {
				md += "\n**Recommendation:** Not yet ready. Continue discovery cycles to gather more evidence.\n"
			}
		} else if currentMode == "outcome" {
			md += "\n### Transition Readiness: Outcome → Scale\n\n"
			md += "Requires consistent user demand, repeated product usage, early revenue, and clear product direction.\n"
			md += "Use feature workflow metrics and business signals to assess readiness.\n"
		}

		// Unused variable suppression
		_ = invalidated

		return helpers.TextResult(md), nil
	}
}
