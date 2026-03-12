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

func GetDiscoveryStatusSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"cycle_id":   map[string]any{"type": "string", "description": "Optionally scope to a specific cycle (DISC-XXX)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

// ---------- Handlers ----------

func GetDiscoveryStatus(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		cycleFilter := helpers.GetString(req.Arguments, "cycle_id")

		// Get project mode
		proj, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}
		mode := string(proj.Mode)
		if mode == "" {
			mode = "outcome"
		}

		var md string
		md += fmt.Sprintf("# Discovery Dashboard — %s\n\n", proj.Name)
		md += fmt.Sprintf("**Mode:** %s\n\n", mode)

		// Active cycles
		cycles, _ := store.ListDiscoveryCycles(ctx, projectID)
		activeCycles := 0
		for _, c := range cycles {
			if c.Status == types.CycleActive {
				activeCycles++
			}
		}
		md += fmt.Sprintf("**Cycles:** %d total (%d active)\n\n", len(cycles), activeCycles)

		// Hypotheses
		hypotheses, _ := store.ListHypotheses(ctx, projectID)
		hypoCounts := map[string]int{}
		for _, h := range hypotheses {
			if cycleFilter != "" && h.CycleID != cycleFilter {
				continue
			}
			hypoCounts[string(h.Status)]++
		}
		totalHypos := 0
		for _, c := range hypoCounts {
			totalHypos += c
		}
		md += fmt.Sprintf("### Hypotheses (%d)\n", totalHypos)
		if totalHypos > 0 {
			md += "| Status | Count |\n|--------|-------|\n"
			for _, s := range types.ValidHypothesisStatuses {
				if c := hypoCounts[s]; c > 0 {
					md += fmt.Sprintf("| %s | %d |\n", s, c)
				}
			}
			md += "\n"
		} else {
			md += "No hypotheses yet.\n\n"
		}

		// Experiments
		experiments, _ := store.ListExperiments(ctx, projectID)
		exprCounts := map[string]int{}
		kindCounts := map[string]int{}
		totalSignals := 0
		killCount := 0
		for _, e := range experiments {
			if cycleFilter != "" && e.CycleID != cycleFilter {
				continue
			}
			exprCounts[string(e.Status)]++
			kindCounts[string(e.Kind)]++
			totalSignals += len(e.Signals)
			if e.KillTriggered {
				killCount++
			}
		}
		totalExprs := 0
		for _, c := range exprCounts {
			totalExprs += c
		}
		md += fmt.Sprintf("### Experiments (%d)\n", totalExprs)
		if totalExprs > 0 {
			md += "| Status | Count |\n|--------|-------|\n"
			for _, s := range types.ValidExperimentStatuses {
				if c := exprCounts[s]; c > 0 {
					md += fmt.Sprintf("| %s | %d |\n", s, c)
				}
			}
			md += "\n**By kind:**\n"
			for _, k := range types.ValidExperimentKinds {
				if c := kindCounts[k]; c > 0 {
					md += fmt.Sprintf("- %s: %d\n", k, c)
				}
			}
			md += "\n"
		} else {
			md += "No experiments yet.\n\n"
		}

		// Signals summary
		md += fmt.Sprintf("### Signals: %d total\n", totalSignals)
		if killCount > 0 {
			md += fmt.Sprintf("**Kill conditions triggered:** %d\n", killCount)
		}
		md += "\n"

		// Spawned features
		spawnedCount := 0
		for _, e := range experiments {
			if cycleFilter != "" && e.CycleID != cycleFilter {
				continue
			}
			spawnedCount += len(e.SpawnedFeatures)
		}
		if spawnedCount > 0 {
			md += fmt.Sprintf("### Features spawned from experiments: %d\n\n", spawnedCount)
		}

		return helpers.TextResult(md), nil
	}
}
