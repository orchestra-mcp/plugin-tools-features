package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
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
			"evidence":   map[string]any{"type": "string", "description": "Evidence for gate transitions. Required for: in-progress->ready-for-testing, in-testing->ready-for-docs, in-docs->documented. Must include required ## sections. Call get_gate_requirements to see what is needed."},
		},
		"required": []any{"project_id", "feature_id"},
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
// next after a transition. This guides the agent through the full lifecycle.
func nextStepHint(featureID string, newStatus types.FeatureStatus) string {
	switch newStatus {
	case types.StatusTodo:
		return fmt.Sprintf("\n\n**Next step:** Call `set_current_feature` or `advance_feature` to start working on **%s**.", featureID)
	case types.StatusInProgress:
		return "\n\n**Next step:** Do the implementation work. When done, call `advance_feature` with evidence (sections: `## Summary`, `## Changes`, `## Verification`)."
	case types.StatusReadyForTesting:
		return fmt.Sprintf("\n\n**Next step:** Call `advance_feature` to move **%s** to in-testing, then run tests.", featureID)
	case types.StatusInTesting:
		return "\n\n**Next step:** Run tests. When done, call `advance_feature` with evidence (sections: `## Summary`, `## Results`, `## Coverage`)."
	case types.StatusReadyForDocs:
		return fmt.Sprintf("\n\n**Next step:** Call `advance_feature` to move **%s** to in-docs, then write documentation.", featureID)
	case types.StatusInDocs:
		return "\n\n**Next step:** Write documentation. When done, call `advance_feature` with evidence (sections: `## Summary`, `## Location`)."
	case types.StatusDocumented:
		return fmt.Sprintf("\n\n**Next step:** Call `request_review` with self-review evidence (sections: `## Summary`, `## Quality`, `## Checklist`). Then ask the user for approval via `AskUserQuestion`.")
	case types.StatusInReview:
		return "\n\n**Next step:** Ask the user for approval via `AskUserQuestion`, then call `submit_review` with their decision."
	case types.StatusNeedsEdits:
		return fmt.Sprintf("\n\n**Next step:** Call `set_current_feature` to restart work on **%s**, then address the feedback.", featureID)
	case types.StatusDone:
		return fmt.Sprintf("\n\n**%s** is complete! Call `get_next_feature` to pick up the next task.", featureID)
	default:
		return ""
	}
}

// ---------- Handlers ----------

// MinGateInterval is the minimum time that must pass between gated transitions.
// This prevents agents from rapid-fire advancing through gates without doing
// actual work (testing, documentation, review). Set to 30 seconds — enough to
// catch instant batch-advancement but short enough for legitimate fast work.
// Exported as a variable so tests can override it.
var MinGateInterval = 30 * time.Second

// AdvanceFeature advances a feature to the next valid status in the workflow.
// Gated transitions require structured evidence with specific ## sections.
// Call get_gate_requirements to see what is needed for the next transition.
//
// GUARDRAIL: Gated transitions enforce a minimum time interval since the last
// status change. This prevents agents from skipping actual work by advancing
// through all gates in seconds.
func AdvanceFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		evidence := helpers.GetString(req.Arguments, "evidence")

		feat, body, version, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Block advance_feature from in-review; must use submit_review instead.
		if feat.Status == types.StatusInReview {
			return helpers.ErrorResult("gate_blocked",
				"Cannot advance from **in-review** using advance_feature. Use the **submit_review** tool to approve or reject. The user must approve the review via AskUserQuestion before calling submit_review."), nil
		}

		nextStatuses := types.NextStatuses(feat.Status)
		if len(nextStatuses) == 0 {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("feature %s is in terminal status %q and cannot be advanced", featureID, feat.Status)), nil
		}

		// Take the first valid transition (the "happy path").
		newStatus := nextStatuses[0]
		oldStatus := feat.Status

		// Check if this transition is gated.
		gate := types.GetGate(oldStatus, newStatus)
		if gate != nil {
			if gate.IsSkippableFor(feat.Kind) {
				// Auto-pass: append a note that the gate was skipped for this kind.
				body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s): Gate skipped for kind=%s\n", oldStatus, newStatus, helpers.NowISO(), feat.Kind)
			} else {
				// GUARDRAIL: Enforce minimum time between gated transitions.
				if elapsed, ok := timeSinceUpdate(feat.UpdatedAt); ok && elapsed < MinGateInterval {
					remaining := MinGateInterval - elapsed
					return helpers.ErrorResult("gate_cooldown",
						fmt.Sprintf("## Gate Cooldown\n\n"+
							"Cannot advance **%s** yet — only **%s** since the last status change.\n\n"+
							"Gated transitions require at least **%s** between advances to ensure "+
							"actual work (testing, documentation, review) is performed.\n\n"+
							"Wait **%s** before trying again, or do the required work first.",
							featureID,
							elapsed.Round(time.Second),
							MinGateInterval,
							remaining.Round(time.Second))), nil
				}

				if err := gate.Validate(evidence); err != nil {
					msg := fmt.Sprintf("## Gate Blocked: %s\n\n**%s**\n\n%s",
						gate.Name, err.Error(), gate.Checklist)
					return helpers.ErrorResult("gate_blocked", msg), nil
				}
				// Append evidence to body.
				body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s):\n%s\n", oldStatus, newStatus, helpers.NowISO(), evidence)
			}
		} else if evidence != "" {
			// Free transition but evidence was provided anyway.
			body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s):\n%s\n", oldStatus, newStatus, helpers.NowISO(), evidence)
		}

		feat.Status = newStatus
		feat.UpdatedAt = helpers.NowISO()

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		msg := fmt.Sprintf("Advanced **%s** from **%s** to **%s**", featureID, oldStatus, newStatus)
		msg += nextStepHint(featureID, newStatus)
		return helpers.TextResult(msg), nil
	}
}

// timeSinceUpdate parses the feature's UpdatedAt ISO timestamp and returns the
// duration since that time. Returns (0, false) if the timestamp is empty or
// unparseable, allowing the transition to proceed (fail-open for legacy data).
func timeSinceUpdate(updatedAt string) (time.Duration, bool) {
	if updatedAt == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		// Try alternate ISO format without timezone.
		t, err = time.Parse("2006-01-02T15:04:05", updatedAt)
		if err != nil {
			return 0, false
		}
	}
	return time.Since(t), true
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

		// Default: find features in "todo" status.
		if statusFilter == "" {
			statusFilter = string(types.StatusTodo)
		}

		// Priority order: P0 > P1 > P2 > P3.
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
// If the feature is in backlog, it auto-advances through todo first.
// GUARDRAIL: Blocks if the same assignee (or unassigned) already has an active
// feature. Different assignees (parallel agents) can each work on one feature.
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

		// GUARDRAIL: Model capability check.
		// If the agent declares its model and the feature has an estimate,
		// verify the model can handle features of that size.
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
		// An agent can only have one active feature. If the target feature has an
		// assignee, we check for other active features with that same assignee.
		// If unassigned, we check for any unassigned active features.
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
			// Same assignee scope: both unassigned, or both assigned to the same person.
			if feat.Assignee == f.Assignee {
				assigneeMsg := "unassigned"
				if feat.Assignee != "" {
					assigneeMsg = fmt.Sprintf("assignee **%s**", feat.Assignee)
				}
				return helpers.ErrorResult("wip_violation",
					fmt.Sprintf("Cannot start **%s** — feature **%s** (%s) is already **%s** for %s. "+
						"Finish it through its full lifecycle (→ done) before starting another feature. "+
						"One feature at a time per agent/assignee.",
						featureID, f.ID, f.Title, f.Status, assigneeMsg)), nil
			}
		}

		// Auto-advance from backlog → todo so the next transition to in-progress is valid.
		if feat.Status == types.StatusBacklog {
			feat.Status = types.StatusTodo
			feat.UpdatedAt = helpers.NowISO()
		}

		if !types.CanTransition(feat.Status, types.StatusInProgress) {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("cannot set to in-progress from status %q", feat.Status)), nil
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

// isActiveStatus returns true if the feature is in a "work in progress" state
// (anywhere between in-progress and in-review, inclusive). Features in these
// states must be completed before another feature can be started.
func isActiveStatus(s types.FeatureStatus) bool {
	switch s {
	case types.StatusInProgress, types.StatusReadyForTesting, types.StatusInTesting,
		types.StatusReadyForDocs, types.StatusInDocs, types.StatusDocumented,
		types.StatusInReview:
		return true
	}
	return false
}

// modelTier maps model name patterns to capability tiers.
// Higher tier = more capable. Tier determines max feature estimate.
type modelTier struct {
	pattern     string // substring to match in model name
	tier        int    // 1=small, 2=medium, 3=large
	displayName string
}

// Model capability tiers — maps model patterns to max estimate they can handle.
// Tier 1 (Haiku-class): S only
// Tier 2 (Sonnet-class): S, M
// Tier 3 (Opus-class): S, M, L, XL
var modelTiers = []modelTier{
	// Opus-class (tier 3)
	{pattern: "opus", tier: 3, displayName: "Opus"},
	// Sonnet-class (tier 2)
	{pattern: "sonnet", tier: 2, displayName: "Sonnet"},
	// Haiku-class (tier 1)
	{pattern: "haiku", tier: 1, displayName: "Haiku"},
	// GPT-4 class (tier 3)
	{pattern: "gpt-4o", tier: 2, displayName: "GPT-4o"},
	{pattern: "gpt-4", tier: 3, displayName: "GPT-4"},
	// GPT-3.5 class (tier 1)
	{pattern: "gpt-3", tier: 1, displayName: "GPT-3.5"},
	// Gemini
	{pattern: "gemini-ultra", tier: 3, displayName: "Gemini Ultra"},
	{pattern: "gemini-pro", tier: 2, displayName: "Gemini Pro"},
	{pattern: "gemini-flash", tier: 1, displayName: "Gemini Flash"},
}

// estimateTierRequired maps estimate sizes to minimum model tier.
var estimateTierRequired = map[string]int{
	"S":  1, // any model
	"M":  2, // sonnet or better
	"L":  3, // opus or better
	"XL": 3, // opus or better
}

// validateModelCapability checks if the given model can handle a feature of the
// given estimate size. Returns an error if the model is too small.
func validateModelCapability(model, estimate string) error {
	requiredTier, ok := estimateTierRequired[estimate]
	if !ok {
		return nil // unknown estimate, allow
	}

	// Find the model's tier by substring matching.
	modelLower := strings.ToLower(model)
	for _, mt := range modelTiers {
		if strings.Contains(modelLower, mt.pattern) {
			if mt.tier < requiredTier {
				tierNames := map[int]string{1: "S only", 2: "S, M", 3: "S, M, L, XL"}
				return fmt.Errorf(
					"Model **%s** (%s-class, handles %s) is not capable enough for estimate **%s**. "+
						"Features sized %s require a tier %d model (e.g., %s).",
					model, mt.displayName, tierNames[mt.tier],
					estimate, estimate, requiredTier,
					tierSuggestion(requiredTier))
			}
			return nil // model is capable enough
		}
	}

	return nil // unknown model, allow (fail-open)
}

func tierSuggestion(tier int) string {
	switch tier {
	case 2:
		return "Sonnet, GPT-4o, Gemini Pro"
	case 3:
		return "Opus, GPT-4, Gemini Ultra"
	default:
		return "any model"
	}
}

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

// GetGateRequirements returns the gate requirements for the next transition of
// a feature. If the next transition is free (no gate), it says so. If the
// feature is in-review, it directs to submit_review.
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

		// Terminal status.
		nextStatuses := types.NextStatuses(feat.Status)
		if len(nextStatuses) == 0 {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is in terminal status **%s** and cannot be advanced.",
				featureID, feat.Status)), nil
		}

		// In-review: must use submit_review.
		if feat.Status == types.StatusInReview {
			return helpers.TextResult(fmt.Sprintf(
				`Feature **%s** is **in-review**.

Use the **submit_review** tool (not advance_feature) to approve or reject.

**Important:** You MUST ask the user for approval via **AskUserQuestion** before calling submit_review.
Present the feature details and self-review evidence, then call submit_review with the user's decision.`,
				featureID)), nil
		}

		// Documented: must use request_review.
		if feat.Status == types.StatusDocumented {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is **documented**. Use the **request_review** tool to request a human review.\n\n%s",
				featureID, types.ReviewGate.Checklist)), nil
		}

		nextStatus := nextStatuses[0]
		gate := types.GetGate(feat.Status, nextStatus)

		if gate == nil {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is **%s**. The next transition to **%s** is **free** — you can call advance_feature without evidence.",
				featureID, feat.Status, nextStatus)), nil
		}

		return helpers.TextResult(fmt.Sprintf(
			"Feature **%s** is **%s**. The next transition to **%s** requires passing a gate:\n\n%s",
			featureID, feat.Status, nextStatus, gate.Checklist)), nil
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
