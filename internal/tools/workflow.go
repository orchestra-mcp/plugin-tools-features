package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func AdvanceFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"evidence":   map[string]any{"type": "string", "description": "Evidence with file paths proving the previous phase is complete. Required section depends on current status: ## Changes (from in-progress), ## Results (from in-testing), ## Docs (from in-docs)."},
			"force":      map[string]any{"type": "boolean", "description": "Force advance even if file types don't match expected patterns (use after user approval via AskUserQuestion)"},
		},
		"required": []any{"project_id", "feature_id", "evidence"},
	})
	return s
}

func RejectFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"reason":     map[string]any{"type": "string", "description": "Reason for rejection"},
		},
		"required": []any{"project_id", "feature_id", "reason"},
	})
	return s
}

func GetNextFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)"},
			"assignee":   map[string]any{"type": "string", "description": "Filter by assignee (optional)"},
			"kind":       map[string]any{"type": "string", "description": "Filter by feature kind (optional)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func SetCurrentFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
			"model":      map[string]any{"type": "string", "description": "The AI model being used (e.g., claude-opus-4-6, claude-sonnet-4-6, claude-haiku-4-5). Used to validate that the feature's size estimate is within the model's capability."},
		},
		"required": []any{"project_id", "feature_id"},
	})
	return s
}

func GetWorkflowStatusSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

// ---------- Helpers ----------

// nextStepHint returns a markdown instruction for what the agent should do
// next after a transition. Each status = exactly one activity.
func nextStepHint(featureID string, newStatus types.FeatureStatus) string {
	switch newStatus {
	case types.StatusTodo:
		return fmt.Sprintf("\n\n**Next step:** Call `set_current_feature` to start working on **%s**.", featureID)
	case types.StatusInProgress:
		return "\n\n**ALLOWED:** Write source code ONLY. Do NOT write tests. Do NOT write docs.\n\n**When done coding:** Call `advance_feature` with evidence listing files changed (section: `## Changes`)."
	case types.StatusInTesting:
		return "\n\n**ALLOWED:** Write test code and run tests ONLY. Do NOT write source code. Do NOT write docs.\n\n**When done testing:** Call `advance_feature` with evidence listing test files and results (section: `## Results`)."
	case types.StatusInDocs:
		return "\n\n**ALLOWED:** Write .md files in `/docs` folder ONLY. Do NOT write source code. Do NOT write tests.\n\n**When done documenting:** Call `advance_feature` with evidence listing doc files (section: `## Docs`)."
	case types.StatusInReview:
		return "\n\n**ALLOWED:** Ask user for approval via `AskUserQuestion` ONLY. Do NOTHING else.\n\n**After user responds:** Call `submit_review` with their decision."
	case types.StatusNeedsEdits:
		return fmt.Sprintf("\n\n**Next step:** Call `set_current_feature` to restart work on **%s**, then address the feedback.", featureID)
	case types.StatusDone:
		return fmt.Sprintf("\n\n**%s** is complete! Call `get_next_feature` to pick up the next task.", featureID)
	default:
		return ""
	}
}

// ---------- Handlers ----------

// AdvanceFeature advances a feature to the next valid status in the workflow.
// Every transition requires evidence with file paths proving the previous phase
// is complete. File-type validation ensures test gates reference test files and
// docs gates reference .md files in the docs/ folder.
func AdvanceFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "evidence"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		evidence := helpers.GetString(req.Arguments, "evidence")
		force := helpers.GetBool(req.Arguments, "force")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Migrate legacy statuses.
		feat.Status = types.MigrateStatus(feat.Status)

		// Block advance_feature from in-review; must use submit_review instead.
		if feat.Status == types.StatusInReview {
			return helpers.ErrorResult("gate_blocked",
				"Cannot advance from **in-review** using advance_feature. Use **submit_review** to approve or reject. Ask the user first via AskUserQuestion."), nil
		}

		// SESSION LOCK CHECK: verify this session owns the feature.
		sessionID := req.GetSessionId()
		if sessionID != "" {
			if err := globaldb.CheckLock(projectID, featureID, sessionID); err != nil {
				return helpers.ErrorResult("session_lock", err.Error()), nil
			}
			globaldb.RefreshLock(projectID, featureID, sessionID)
		}

		nextStatuses := types.NextStatuses(feat.Status)
		if len(nextStatuses) == 0 {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("feature %s is in terminal status %q and cannot be advanced", featureID, feat.Status)), nil
		}

		// Determine target status.
		newStatus := nextStatuses[0]
		oldStatus := feat.Status

		// For bugs/hotfixes/testcases in-testing: skip docs, go to in-review.
		if oldStatus == types.StatusInTesting && len(nextStatuses) > 1 {
			kind := feat.Kind
			if kind == "" {
				kind = types.KindFeature
			}
			if kind == types.KindBug || kind == types.KindHotfix || kind == types.KindTestcase {
				newStatus = types.StatusInReview
			}
		}

		// Check gate.
		gate := types.GetGate(oldStatus, newStatus)
		if gate != nil {
			if gate.IsSkippableFor(feat.Kind) {
				body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s): Gate skipped for kind=%s\n", oldStatus, newStatus, helpers.NowISO(), feat.Kind)
			} else {
				// Validate evidence structure.
				if err := gate.Validate(evidence); err != nil {
					return helpers.ErrorResult("gate_blocked",
						fmt.Sprintf("## Gate Blocked: %s\n\n%s", gate.Name, err.Error())), nil
				}

				// File-type validation (unless force=true after user approval).
				if !force {
					ok, expected := gate.CheckFileTypes(evidence)
					if !ok {
						return helpers.ErrorResult("needs_approval",
							fmt.Sprintf("## File Type Mismatch\n\n"+
								"Evidence for **%s** gate references files that don't match expected patterns.\n\n"+
								"**Expected:** %s\n\n"+
								"Ask the user to confirm via `AskUserQuestion`, then retry with `force: true`.",
								gate.Name, strings.Join(expected, ", "))), nil
					}
				}

				// Append evidence to body.
				body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s):\n%s\n", oldStatus, newStatus, helpers.NowISO(), evidence)
			}
		}

		feat.Status = newStatus
		feat.UpdatedAt = helpers.NowISO()

		// Release session lock when feature reaches done.
		if newStatus == types.StatusDone && sessionID != "" {
			globaldb.ReleaseLock(projectID, featureID)
		}

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		msg := fmt.Sprintf("Advanced **%s** from **%s** to **%s**", featureID, oldStatus, newStatus)
		msg += nextStepHint(featureID, newStatus)
		return helpers.TextResult(msg), nil
	}
}

// RejectFeature sets a feature's status to needs-edits.
func RejectFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "reason"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		reason := helpers.GetString(req.Arguments, "reason")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if !types.CanTransition(feat.Status, types.StatusNeedsEdits) {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("cannot reject feature from status %q", feat.Status)), nil
		}

		oldStatus := feat.Status
		feat.Status = types.StatusNeedsEdits
		feat.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n\n---\n**Rejected (%s -> needs-edits)** (%s): %s\n", oldStatus, helpers.NowISO(), reason)

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		msg := fmt.Sprintf("Rejected **%s** (%s -> needs-edits): %s", featureID, oldStatus, reason)
		msg += nextStepHint(featureID, types.StatusNeedsEdits)
		return helpers.TextResult(msg), nil
	}
}

// GetNextFeature returns the next feature to work on based on filters.
func GetNextFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")
		assigneeFilter := helpers.GetString(req.Arguments, "assignee")
		kindFilter := helpers.GetString(req.Arguments, "kind")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if statusFilter == "" {
			statusFilter = string(types.StatusTodo)
		}

		priorityRank := map[string]int{"P0": 0, "P1": 1, "P2": 2, "P3": 3}

		var best *types.FeatureData
		bestRank := 999

		for _, f := range features {
			if string(f.Status) != statusFilter {
				continue
			}
			if assigneeFilter != "" && f.Assignee != assigneeFilter {
				continue
			}
			if kindFilter != "" {
				k := string(f.Kind)
				if k == "" {
					k = "feature"
				}
				if k != kindFilter {
					continue
				}
			}
			rank, ok := priorityRank[f.Priority]
			if !ok {
				rank = 99
			}
			if best == nil || rank < bestRank {
				best = f
				bestRank = rank
			}
		}

		if best == nil {
			return helpers.TextResult("No features found matching the criteria."), nil
		}

		md := fmt.Sprintf("**Next feature:**\n\n%s", helpers.FormatFeatureMD(best))
		md += fmt.Sprintf("\n**Next step:** Call `set_current_feature` with feature_id `%s` to start working on it.", best.ID)
		return helpers.TextResult(md), nil
	}
}

// SetCurrentFeature sets a feature's status to in-progress.
// Only valid from todo or needs-edits status.
//
// GUARDRAILS:
// - Model capability check (S/M/L/XL vs model tier)
// - One feature at a time per assignee
// - Session lock acquisition
func SetCurrentFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		model := helpers.GetString(req.Arguments, "model")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Migrate legacy statuses.
		feat.Status = types.MigrateStatus(feat.Status)

		// GUARDRAIL: Model capability check.
		if model != "" && feat.Estimate != "" {
			if err := validateModelCapability(model, feat.Estimate); err != nil {
				return helpers.ErrorResult("model_capability",
					fmt.Sprintf("## Model Capability Warning\n\n%s\n\n"+
						"Either use a more capable model, or break this feature into smaller pieces "+
						"(use `create_plan` + `breakdown_plan` to split into S/M features).",
						err.Error())), nil
			}
		}

		// GUARDRAIL: One feature at a time per assignee.
		allFeatures, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}
		for _, f := range allFeatures {
			if f.ID == featureID {
				continue
			}
			if !isActiveStatus(f.Status) {
				continue
			}
			if feat.Assignee == f.Assignee {
				assigneeMsg := "unassigned"
				if feat.Assignee != "" {
					assigneeMsg = fmt.Sprintf("assignee **%s**", feat.Assignee)
				}
				return helpers.ErrorResult("wip_violation",
					fmt.Sprintf("Cannot start **%s** -- feature **%s** (%s) is already **%s** for %s. "+
						"Finish it through its full lifecycle (-> done) before starting another feature.",
						featureID, f.ID, f.Title, f.Status, assigneeMsg)), nil
			}
		}

		if !types.CanTransition(feat.Status, types.StatusInProgress) {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("cannot set to in-progress from status %q — feature must be in 'todo' or 'needs-edits'", feat.Status)), nil
		}

		// SESSION LOCK: acquire exclusive lock for this session.
		sessionID := req.GetSessionId()
		if sessionID != "" {
			if err := globaldb.AcquireLock(projectID, featureID, sessionID); err != nil {
				return helpers.ErrorResult("session_lock",
					fmt.Sprintf("Cannot start **%s** -- it is locked by another session. "+
						"Wait for the other session to finish or call `unlock_feature` to force-release. %v",
						featureID, err)), nil
			}
		}

		oldStatus := feat.Status
		feat.Status = types.StatusInProgress
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		msg := fmt.Sprintf("Set **%s** to **in-progress** (was %s)\n\n", featureID, oldStatus)
		msg += helpers.FormatFeatureMD(feat)
		if body != "" {
			msg += "\n---\n\n" + body
		}
		msg += nextStepHint(featureID, types.StatusInProgress)
		return helpers.TextResult(msg), nil
	}
}

// isActiveStatus returns true if the feature is in a "work in progress" state.
func isActiveStatus(s types.FeatureStatus) bool {
	switch s {
	case types.StatusInProgress, types.StatusInTesting, types.StatusInDocs, types.StatusInReview:
		return true
	}
	return false
}

// ---------- Model capability validation ----------

type modelTier struct {
	pattern     string
	tier        int
	displayName string
}

var modelTiers = []modelTier{
	{pattern: "opus", tier: 3, displayName: "Opus"},
	{pattern: "sonnet", tier: 2, displayName: "Sonnet"},
	{pattern: "haiku", tier: 1, displayName: "Haiku"},
	{pattern: "gpt-4o", tier: 2, displayName: "GPT-4o"},
	{pattern: "gpt-4", tier: 3, displayName: "GPT-4"},
	{pattern: "gpt-3", tier: 1, displayName: "GPT-3.5"},
	{pattern: "gemini-ultra", tier: 3, displayName: "Gemini Ultra"},
	{pattern: "gemini-pro", tier: 2, displayName: "Gemini Pro"},
	{pattern: "gemini-flash", tier: 1, displayName: "Gemini Flash"},
}

var estimateTierRequired = map[string]int{
	"S": 1, "M": 2, "L": 3, "XL": 3,
}

func validateModelCapability(model, estimate string) error {
	requiredTier, ok := estimateTierRequired[estimate]
	if !ok {
		return nil
	}

	modelLower := strings.ToLower(model)
	for _, mt := range modelTiers {
		if strings.Contains(modelLower, mt.pattern) {
			if mt.tier < requiredTier {
				tierNames := map[int]string{1: "S only", 2: "S, M", 3: "S, M, L, XL"}
				return fmt.Errorf(
					"Model **%s** (%s-class, handles %s) is not capable enough for estimate **%s**.",
					model, mt.displayName, tierNames[mt.tier], estimate)
			}
			return nil
		}
	}

	return nil
}

// ---------- Gate Requirements ----------

func GetGateRequirementsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID"},
		},
		"required": []any{"project_id", "feature_id"},
	})
	return s
}

func GetGateRequirements(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")

		feat, _, _, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		feat.Status = types.MigrateStatus(feat.Status)

		nextStatuses := types.NextStatuses(feat.Status)
		if len(nextStatuses) == 0 {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is in terminal status **%s**.", featureID, feat.Status)), nil
		}

		if feat.Status == types.StatusInReview {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is **in-review**. Use `submit_review` (not advance_feature). "+
					"You MUST ask the user via `AskUserQuestion` first.", featureID)), nil
		}

		nextStatus := nextStatuses[0]
		gate := types.GetGate(feat.Status, nextStatus)

		if gate == nil {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is **%s**. Next transition to **%s** is free — call advance_feature.",
				featureID, feat.Status, nextStatus)), nil
		}

		msg := fmt.Sprintf("Feature **%s** is **%s**. Next transition to **%s** requires:\n\n"+
			"- Section: `## %s`\n"+
			"- Must include at least %d file path(s)\n",
			featureID, feat.Status, nextStatus, gate.RequiredSection, gate.MinFilePaths)

		if len(gate.FilePatterns) > 0 {
			msg += fmt.Sprintf("- File types expected: %s\n", strings.Join(gate.FilePatterns, ", "))
			msg += "- If files don't match patterns, user approval is needed (force: true)\n"
		}
		if gate.DocsFolder != "" {
			msg += fmt.Sprintf("- Files must be in `%s/` folder\n", gate.DocsFolder)
		}

		return helpers.TextResult(msg), nil
	}
}

// GetWorkflowStatus returns feature counts per status for a project.
func GetWorkflowStatus(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		statusCounts := make(map[string]int)
		for _, f := range features {
			statusCounts[string(f.Status)]++
		}

		md := fmt.Sprintf("## Workflow Status: %s\n\n", projectID) + helpers.FormatStatusCountsMD(statusCounts, len(features))
		return helpers.TextResult(md), nil
	}
}
