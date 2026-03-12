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

func CreateExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project slug"},
			"hypothesis_id":  map[string]any{"type": "string", "description": "Hypothesis this experiment tests (HYPO-XXX)"},
			"title":          map[string]any{"type": "string", "description": "Experiment title"},
			"kind":           map[string]any{"type": "string", "description": "Experiment type", "enum": []any{"interview", "landing-page", "prototype", "concierge", "survey", "ab-test", "mock", "other"}},
			"question":       map[string]any{"type": "string", "description": "What we want to learn (e.g. 'Would founders upload contracts to get automated risk insights?')"},
			"method":         map[string]any{"type": "string", "description": "How we will test this"},
			"success_signal": map[string]any{"type": "string", "description": "What would validate the hypothesis (e.g. '5 of 10 founders upload a real contract')"},
			"kill_condition": map[string]any{"type": "string", "description": "When to abandon this direction (e.g. 'fewer than 3 users show interest')"},
			"cycle_id":       map[string]any{"type": "string", "description": "Link to a discovery cycle (DISC-XXX)"},
			"labels":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional labels"},
		},
		"required": []any{"project_id", "hypothesis_id", "title", "kind", "question", "method", "success_signal", "kill_condition"},
	})
	return s
}

func GetExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id": map[string]any{"type": "string", "description": "Experiment ID (EXPR-XXX)"},
		},
		"required": []any{"project_id", "experiment_id"},
	})
	return s
}

func ListExperimentsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"status":        map[string]any{"type": "string", "description": "Filter by status", "enum": []any{"draft", "running", "completed", "abandoned"}},
			"hypothesis_id": map[string]any{"type": "string", "description": "Filter by hypothesis (HYPO-XXX)"},
			"cycle_id":      map[string]any{"type": "string", "description": "Filter by cycle (DISC-XXX)"},
			"kind":          map[string]any{"type": "string", "description": "Filter by experiment kind"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func UpdateExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id":  map[string]any{"type": "string", "description": "Experiment ID (EXPR-XXX)"},
			"title":          map[string]any{"type": "string", "description": "New title"},
			"method":         map[string]any{"type": "string", "description": "New method"},
			"success_signal": map[string]any{"type": "string", "description": "New success signal"},
			"kill_condition": map[string]any{"type": "string", "description": "New kill condition"},
		},
		"required": []any{"project_id", "experiment_id"},
	})
	return s
}

func StartExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id": map[string]any{"type": "string", "description": "Experiment ID (EXPR-XXX)"},
		},
		"required": []any{"project_id", "experiment_id"},
	})
	return s
}

func RecordSignalSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id": map[string]any{"type": "string", "description": "Experiment ID (EXPR-XXX)"},
			"signal_type":   map[string]any{"type": "string", "description": "Signal category", "enum": []any{"user", "behavior", "market"}},
			"metric":        map[string]any{"type": "string", "description": "What was measured (e.g. 'contract upload rate')"},
			"expected":      map[string]any{"type": "string", "description": "What we expected (e.g. '50% of users')"},
			"actual":        map[string]any{"type": "string", "description": "What we observed (e.g. '20% of users')"},
			"confidence":    map[string]any{"type": "string", "description": "Confidence level", "enum": []any{"low", "medium", "high"}},
		},
		"required": []any{"project_id", "experiment_id", "signal_type", "metric", "expected", "actual", "confidence"},
	})
	return s
}

func CompleteExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id": map[string]any{"type": "string", "description": "Experiment ID (EXPR-XXX)"},
			"outcome":       map[string]any{"type": "string", "description": "Summary of experiment results and conclusions"},
		},
		"required": []any{"project_id", "experiment_id", "outcome"},
	})
	return s
}

func AbandonExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":     map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id":  map[string]any{"type": "string", "description": "Experiment ID (EXPR-XXX)"},
			"reason":         map[string]any{"type": "string", "description": "Why the experiment was abandoned"},
			"kill_triggered": map[string]any{"type": "boolean", "description": "Whether the kill condition was met"},
		},
		"required": []any{"project_id", "experiment_id", "reason"},
	})
	return s
}

func SpawnFeatureFromExperimentSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"experiment_id": map[string]any{"type": "string", "description": "Source experiment (EXPR-XXX)"},
			"title":         map[string]any{"type": "string", "description": "Feature title"},
			"description":   map[string]any{"type": "string", "description": "Feature description"},
			"priority":      map[string]any{"type": "string", "description": "Priority (P0-P3)", "enum": []any{"P0", "P1", "P2", "P3"}},
			"kind":          map[string]any{"type": "string", "description": "Feature kind", "enum": []any{"feature", "bug", "hotfix", "chore", "testcase"}},
		},
		"required": []any{"project_id", "experiment_id", "title"},
	})
	return s
}

// ---------- Handlers ----------

func CreateExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "hypothesis_id", "title", "kind", "question", "method", "success_signal", "kill_condition"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		hypoID := helpers.GetString(req.Arguments, "hypothesis_id")
		kind := helpers.GetString(req.Arguments, "kind")

		if _, _, err := store.ReadProject(ctx, projectID); err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}
		if _, _, _, err := store.ReadHypothesis(ctx, projectID, hypoID); err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("hypothesis %q not found", hypoID)), nil
		}
		if err := helpers.ValidateOneOf(kind, types.ValidExperimentKinds...); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		now := helpers.NowISO()
		exprID := helpers.NewExperimentID()
		expr := &types.ExperimentData{
			ID:            exprID,
			ProjectID:     projectID,
			HypothesisID:  hypoID,
			CycleID:       helpers.GetString(req.Arguments, "cycle_id"),
			Title:         helpers.GetString(req.Arguments, "title"),
			Kind:          types.ExperimentKind(kind),
			Question:      helpers.GetString(req.Arguments, "question"),
			Method:        helpers.GetString(req.Arguments, "method"),
			SuccessSignal: helpers.GetString(req.Arguments, "success_signal"),
			KillCondition: helpers.GetString(req.Arguments, "kill_condition"),
			Status:        types.ExprDraft,
			Labels:        helpers.GetStringSlice(req.Arguments, "labels"),
			Version:       0,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		body := fmt.Sprintf("# %s\n\n**Hypothesis:** %s\n**Kind:** %s\n\n**Question:** %s\n\n**Method:** %s\n\n**Success Signal:** %s\n\n**Kill Condition:** %s\n",
			expr.Title, hypoID, kind, expr.Question, expr.Method, expr.SuccessSignal, expr.KillCondition)

		if _, err := store.WriteExperiment(ctx, projectID, exprID, expr, body, 0); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Link to hypothesis and cycle
		linkExperimentToHypothesis(ctx, store, projectID, hypoID, exprID)
		if expr.CycleID != "" {
			linkExperimentToCycle(ctx, store, projectID, expr.CycleID, exprID)
		}

		md := fmt.Sprintf("Created experiment **%s**: %s\n\n%s", exprID, expr.Title,
			helpers.FormatExperimentMD(expr))
		return helpers.TextResult(md), nil
	}
}

func GetExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")

		expr, body, _, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}

		md := helpers.FormatExperimentMD(expr)
		if body != "" {
			md += "\n---\n" + body
		}
		return helpers.TextResult(md), nil
	}
}

func ListExperiments(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")
		hypoFilter := helpers.GetString(req.Arguments, "hypothesis_id")
		cycleFilter := helpers.GetString(req.Arguments, "cycle_id")
		kindFilter := helpers.GetString(req.Arguments, "kind")

		experiments, err := store.ListExperiments(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var filtered []*types.ExperimentData
		for _, e := range experiments {
			if statusFilter != "" && string(e.Status) != statusFilter {
				continue
			}
			if hypoFilter != "" && e.HypothesisID != hypoFilter {
				continue
			}
			if cycleFilter != "" && e.CycleID != cycleFilter {
				continue
			}
			if kindFilter != "" && string(e.Kind) != kindFilter {
				continue
			}
			filtered = append(filtered, e)
		}

		return helpers.TextResult(helpers.FormatExperimentListMD(filtered, "Experiments")), nil
	}
}

func UpdateExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")

		expr, body, version, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}
		if expr.Status != types.ExprDraft {
			return helpers.ErrorResult("invalid_state", "can only update experiments in draft status"), nil
		}

		if t := helpers.GetString(req.Arguments, "title"); t != "" {
			expr.Title = t
		}
		if m := helpers.GetString(req.Arguments, "method"); m != "" {
			expr.Method = m
		}
		if ss := helpers.GetString(req.Arguments, "success_signal"); ss != "" {
			expr.SuccessSignal = ss
		}
		if kc := helpers.GetString(req.Arguments, "kill_condition"); kc != "" {
			expr.KillCondition = kc
		}
		expr.UpdatedAt = helpers.NowISO()

		if _, err := store.WriteExperiment(ctx, projectID, exprID, expr, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Updated experiment **%s**\n\n%s", exprID,
			helpers.FormatExperimentMD(expr))), nil
	}
}

func StartExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")

		expr, body, version, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}
		if expr.Status != types.ExprDraft {
			return helpers.ErrorResult("invalid_state", fmt.Sprintf("experiment is %s, not draft", expr.Status)), nil
		}

		expr.Status = types.ExprRunning
		expr.UpdatedAt = helpers.NowISO()
		body += fmt.Sprintf("\n---\n\n## Started (%s)\n\nExperiment is now running.\n", expr.UpdatedAt)

		if _, err := store.WriteExperiment(ctx, projectID, exprID, expr, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Experiment **%s** is now running.\n\n%s",
			exprID, helpers.FormatExperimentMD(expr))), nil
	}
}

func RecordSignal(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id", "signal_type", "metric", "expected", "actual", "confidence"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")
		signalType := helpers.GetString(req.Arguments, "signal_type")
		confidence := helpers.GetString(req.Arguments, "confidence")

		if err := helpers.ValidateOneOf(signalType, types.ValidSignalTypes...); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		if err := helpers.ValidateOneOf(confidence, "low", "medium", "high"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		expr, body, version, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}
		if expr.Status != types.ExprRunning {
			return helpers.ErrorResult("invalid_state", fmt.Sprintf("experiment is %s, not running", expr.Status)), nil
		}

		now := helpers.NowISO()
		signal := types.ValidationSignal{
			Type:       types.SignalType(signalType),
			Metric:     helpers.GetString(req.Arguments, "metric"),
			Expected:   helpers.GetString(req.Arguments, "expected"),
			Actual:     helpers.GetString(req.Arguments, "actual"),
			Confidence: confidence,
			RecordedAt: now,
		}
		expr.Signals = append(expr.Signals, signal)
		expr.UpdatedAt = now

		body += fmt.Sprintf("\n### Signal (%s)\n- **Type:** %s\n- **Metric:** %s\n- **Expected:** %s\n- **Actual:** %s\n- **Confidence:** %s\n",
			now, signalType, signal.Metric, signal.Expected, signal.Actual, confidence)

		if _, err := store.WriteExperiment(ctx, projectID, exprID, expr, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Recorded signal on **%s** — [%s] %s: expected=%s, actual=%s (confidence: %s)\n\nTotal signals: %d",
			exprID, signalType, signal.Metric, signal.Expected, signal.Actual, confidence, len(expr.Signals))
		return helpers.TextResult(md), nil
	}
}

func CompleteExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id", "outcome"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")
		outcome := helpers.GetString(req.Arguments, "outcome")

		expr, body, version, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}
		if expr.Status != types.ExprRunning {
			return helpers.ErrorResult("invalid_state", fmt.Sprintf("experiment is %s, not running", expr.Status)), nil
		}

		expr.Status = types.ExprCompleted
		expr.Outcome = outcome
		expr.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n---\n\n## Completed (%s)\n\n**Outcome:** %s\n\n**Signals collected:** %d\n",
			expr.UpdatedAt, outcome, len(expr.Signals))

		if _, err := store.WriteExperiment(ctx, projectID, exprID, expr, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Experiment **%s** completed.\n\n**Outcome:** %s\n\n%s",
			exprID, outcome, helpers.FormatExperimentMD(expr))

		// Suggest next steps
		md += "\n\n**Next steps:**\n"
		md += "- `validate_hypothesis` or `invalidate_hypothesis` to update the hypothesis\n"
		md += "- `spawn_feature_from_experiment` to create a feature from validated learnings\n"

		return helpers.TextResult(md), nil
	}
}

func AbandonExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id", "reason"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")
		reason := helpers.GetString(req.Arguments, "reason")
		killTriggered := helpers.GetBool(req.Arguments, "kill_triggered")

		expr, body, version, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}

		expr.Status = types.ExprAbandoned
		expr.Outcome = reason
		expr.KillTriggered = killTriggered
		expr.UpdatedAt = helpers.NowISO()

		killNote := ""
		if killTriggered {
			killNote = " (kill condition triggered)"
		}
		body += fmt.Sprintf("\n---\n\n## Abandoned%s (%s)\n\n%s\n", killNote, expr.UpdatedAt, reason)

		if _, err := store.WriteExperiment(ctx, projectID, exprID, expr, body, version); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Experiment **%s** abandoned%s.\n\n%s",
			exprID, killNote, helpers.FormatExperimentMD(expr))), nil
	}
}

func SpawnFeatureFromExperiment(store *storage.FeatureStorage) func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "experiment_id", "title"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}
		projectID := helpers.GetString(req.Arguments, "project_id")
		exprID := helpers.GetString(req.Arguments, "experiment_id")

		expr, _, exprVersion, err := store.ReadExperiment(ctx, projectID, exprID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("experiment %q not found", exprID)), nil
		}
		if expr.Status != types.ExprCompleted {
			return helpers.ErrorResult("invalid_state", "can only spawn features from completed experiments"), nil
		}

		priority := helpers.GetStringOr(req.Arguments, "priority", "P2")
		kind := helpers.GetStringOr(req.Arguments, "kind", "feature")
		if err := helpers.ValidateOneOf(priority, "P0", "P1", "P2", "P3"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		now := helpers.NowISO()
		featureID := helpers.NewFeatureID()
		description := helpers.GetString(req.Arguments, "description")
		if description == "" {
			description = fmt.Sprintf("Spawned from experiment %s (hypothesis: %s).\n\nOutcome: %s",
				exprID, expr.HypothesisID, expr.Outcome)
		}

		feat := &types.FeatureData{
			ID:          featureID,
			ProjectID:   projectID,
			Title:       helpers.GetString(req.Arguments, "title"),
			Description: description,
			Status:      types.StatusTodo,
			Priority:    priority,
			Kind:        types.FeatureKind(kind),
			Labels:      []string{"experiment:" + exprID, "hypothesis:" + expr.HypothesisID},
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n%s\n\n**Source:** Experiment %s → Hypothesis %s\n",
			feat.Title, description, exprID, expr.HypothesisID)

		if _, err := store.WriteFeature(ctx, projectID, featureID, feat, body, 0); err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Update experiment to track spawned feature
		expr.SpawnedFeatures = append(expr.SpawnedFeatures, featureID)
		expr.UpdatedAt = now
		// Re-read to get latest body
		_, exprBody, _, _ := store.ReadExperiment(ctx, projectID, exprID)
		exprBody += fmt.Sprintf("\n### Spawned Feature (%s)\n- **%s**: %s\n", now, featureID, feat.Title)
		_, _ = store.WriteExperiment(ctx, projectID, exprID, expr, exprBody, exprVersion)

		md := fmt.Sprintf("Spawned feature **%s** from experiment **%s**\n\n%s",
			featureID, exprID, helpers.FormatFeatureMD(feat))
		return helpers.TextResult(md), nil
	}
}
