package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/sdk-go/workflow"
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

// nextStepHintStr is the same as nextStepHint but accepts a plain string state
// (for use with engine-driven workflows where status is not a types.FeatureStatus).
func nextStepHintStr(featureID string, newStatus string) string {
	return nextStepHint(featureID, types.FeatureStatus(newStatus))
}

// ---------- Gate validation helpers (engine-aware) ----------

// filePathPattern matches common file path patterns in evidence text.
var filePathPattern = regexp.MustCompile(`(?:^|[\s` + "`" + `\-*])([.\w][\w./\-]*\.\w{1,10})(?:\s|$|[,:;()\]` + "`" + `])`)

// extractFilePaths pulls unique file paths out of a text block.
func extractFilePaths(text string) []string {
	matches := filePathPattern.FindAllStringSubmatch(text, -1)
	seen := make(map[string]bool)
	var paths []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			paths = append(paths, m[1])
		}
	}
	return paths
}

// parseSections extracts markdown ## sections from evidence text.
func parseSections(text string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(text, "\n")
	var currentSection string
	var currentContent strings.Builder
	for _, line := range lines {
		trimLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimLine, "## ") {
			if currentSection != "" {
				sections[currentSection] = currentContent.String()
			}
			currentSection = strings.ToLower(strings.TrimSpace(trimLine[3:]))
			currentContent.Reset()
		} else if currentSection != "" {
			if currentContent.Len() > 0 || trimLine != "" {
				currentContent.WriteString(line)
				currentContent.WriteString("\n")
			}
		}
	}
	if currentSection != "" {
		sections[currentSection] = currentContent.String()
	}
	return sections
}

// matchesFilePattern checks if a file path matches a glob or suffix pattern.
func matchesFilePattern(path, pattern string) bool {
	lower := strings.ToLower(path)
	lowerPattern := strings.ToLower(pattern)
	if strings.Contains(lowerPattern, "*") {
		matched, _ := filepath.Match(lowerPattern, lower)
		if matched {
			return true
		}
		matched, _ = filepath.Match(lowerPattern, filepath.Base(lower))
		return matched
	}
	return strings.HasSuffix(lower, lowerPattern)
}

// validateGate checks that the evidence satisfies the GateDef requirements.
// Returns a non-nil error if the evidence is missing or malformed.
func validateGate(gate *workflow.GateDef, evidence string) error {
	trimmed := strings.TrimSpace(evidence)
	if trimmed == "" {
		return fmt.Errorf("gate %q requires evidence with a ## %s section listing file paths", gate.Label, gate.RequiredSection)
	}
	sections := parseSections(trimmed)
	content, found := sections[strings.ToLower(gate.RequiredSection)]
	if !found {
		return fmt.Errorf("evidence missing required section: ## %s", gate.RequiredSection)
	}
	if len(strings.TrimSpace(content)) < 10 {
		return fmt.Errorf("section ## %s has insufficient content (minimum 10 characters)", gate.RequiredSection)
	}
	paths := extractFilePaths(content)
	if len(paths) < 1 {
		return fmt.Errorf("section ## %s must reference at least 1 file path(s) (found 0). "+
			"List the actual files changed, e.g.: src/main.go, tests/auth_test.go",
			gate.RequiredSection)
	}
	return nil
}

// checkFileTypes validates that at least one referenced file matches the gate's
// expected file patterns or docs folder constraint.
// Returns (true, nil) when all checks pass or no patterns are configured.
// Returns (false, expectedPatterns) when none of the files match.
func checkFileTypes(gate *workflow.GateDef, evidence string) (ok bool, expected []string) {
	if len(gate.FilePatterns) == 0 && gate.DocsFolder == "" {
		return true, nil
	}
	sections := parseSections(strings.TrimSpace(evidence))
	content := sections[strings.ToLower(gate.RequiredSection)]
	paths := extractFilePaths(content)
	if len(paths) == 0 {
		return false, gate.FilePatterns
	}
	if gate.DocsFolder != "" {
		for _, p := range paths {
			if strings.HasPrefix(p, gate.DocsFolder+"/") || strings.HasPrefix(p, gate.DocsFolder+"\\") {
				if strings.HasSuffix(strings.ToLower(p), ".md") {
					return true, nil
				}
			}
		}
		return false, []string{gate.DocsFolder + "/*.md"}
	}
	for _, p := range paths {
		for _, pattern := range gate.FilePatterns {
			if matchesFilePattern(p, pattern) {
				return true, nil
			}
		}
	}
	return false, gate.FilePatterns
}

// ---------- Handlers ----------

// AdvanceFeature advances a feature to the next valid status in the workflow.
// Every transition requires evidence with file paths proving the previous phase
// is complete. File-type validation ensures test gates reference test files and
// docs gates reference .md files in the docs/ folder.
func AdvanceFeature(store *storage.FeatureStorage, resolver *workflow.EngineResolver) ToolHandler {
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

		// DELEGATION BLOCK CHECK: refuse to advance if feature has pending delegations.
		if delID, toPerson := HasPendingDelegation(ctx, store, projectID, feat); delID != "" {
			return helpers.ErrorResult("delegation_blocked",
				fmt.Sprintf("Feature %s is blocked by pending delegation %s to %s. "+
					"The delegated person must respond before work can continue. "+
					"Use `get_delegation` to check status or `respond_delegation` to answer.",
					featureID, delID, toPerson)), nil
		}

		eng := resolver.Resolve(projectID)
		fromState := workflow.StateID(feat.Status)
		nextStates := eng.NextStates(fromState)
		if len(nextStates) == 0 {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("feature %s is in terminal status %q and cannot be advanced", featureID, feat.Status)), nil
		}

		// Determine target state. Default to first next state.
		toState := nextStates[0]
		oldStatus := feat.Status

		// For features in-testing with multiple next states: pick in-review
		// directly when the feature kind allows skipping docs.
		if len(nextStates) > 1 {
			kind := string(feat.Kind)
			if kind == "" {
				kind = string(types.KindFeature)
			}
			for _, candidate := range nextStates[1:] {
				if eng.IsSkippableFor(fromState, candidate, kind) {
					toState = candidate
					break
				}
			}
		}

		// Check gate for the chosen transition.
		gate := eng.Gate(fromState, toState)
		if gate != nil {
			kind := string(feat.Kind)
			if kind == "" {
				kind = string(types.KindFeature)
			}
			if eng.IsSkippableFor(fromState, toState, kind) {
				body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s): Gate skipped for kind=%s\n", oldStatus, toState, helpers.NowISO(), kind)
			} else {
				// Validate evidence structure.
				if err := validateGate(gate, evidence); err != nil {
					return helpers.ErrorResult("gate_blocked",
						fmt.Sprintf("## Gate Blocked: %s\n\n%s", gate.Label, err.Error())), nil
				}

				// File-type validation (unless force=true after user approval).
				if !force {
					ok, expected := checkFileTypes(gate, evidence)
					if !ok {
						return helpers.ErrorResult("needs_approval",
							fmt.Sprintf("## File Type Mismatch\n\n"+
								"Evidence for **%s** gate references files that don't match expected patterns.\n\n"+
								"**Expected:** %s\n\n"+
								"Ask the user to confirm via `AskUserQuestion`, then retry with `force: true`.",
								gate.Label, strings.Join(expected, ", "))), nil
					}
				}

				body += fmt.Sprintf("\n\n---\n**%s -> %s** (%s):\n%s\n", oldStatus, toState, helpers.NowISO(), evidence)
			}
		}

		feat.Status = types.FeatureStatus(toState)
		feat.UpdatedAt = helpers.NowISO()

		// Release session lock when feature reaches a terminal state.
		if eng.IsTerminal(toState) && sessionID != "" {
			globaldb.ReleaseLock(projectID, featureID)
		}

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		msg := fmt.Sprintf("Advanced **%s** from **%s** to **%s**", featureID, oldStatus, toState)
		msg += nextStepHintStr(featureID, string(toState))
		return helpers.TextResult(msg), nil
	}
}

// RejectFeature sets a feature's status to needs-edits.
func RejectFeature(store *storage.FeatureStorage, resolver *workflow.EngineResolver) ToolHandler {
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

		eng := resolver.Resolve(projectID)
		fromState := workflow.StateID(feat.Status)
		toState := workflow.StateID("needs-edits")
		if !eng.CanTransition(fromState, toState) {
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
// Features locked by other sessions are skipped — they belong to parallel sessions.
func GetNextFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")
		assigneeFilter := helpers.GetString(req.Arguments, "assignee")
		kindFilter := helpers.GetString(req.Arguments, "kind")
		sessionID := req.GetSessionId()

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
			// Skip features locked by a different session — they belong to another
			// parallel IDE session and must not be suggested as next work.
			if sessionID != "" && isActiveStatus(f.Status) {
				if lock, _ := globaldb.GetLockInfo(projectID, f.ID); lock != nil && lock.SessionID != sessionID {
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
func SetCurrentFeature(store *storage.FeatureStorage, resolver *workflow.EngineResolver) ToolHandler {
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
		// Skip features locked by OTHER sessions — those belong to parallel
		// sessions working on the same workspace and don't block this session.
		sessionID := req.GetSessionId()
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
			if feat.Assignee != f.Assignee {
				continue
			}
			// If the conflicting feature is locked by a different session, it
			// belongs to a parallel IDE session — don't block this session.
			if sessionID != "" {
				lock, _ := globaldb.GetLockInfo(projectID, f.ID)
				if lock != nil && lock.SessionID != sessionID {
					continue // Another session owns it — not our WIP.
				}
			}
			assigneeMsg := "unassigned"
			if feat.Assignee != "" {
				assigneeMsg = fmt.Sprintf("assignee **%s**", feat.Assignee)
			}
			return helpers.ErrorResult("wip_violation",
				fmt.Sprintf("Cannot start **%s** -- feature **%s** (%s) is already **%s** for %s. "+
					"Finish it through its full lifecycle (-> done) before starting another feature.",
					featureID, f.ID, f.Title, f.Status, assigneeMsg)), nil
		}

		eng := resolver.Resolve(projectID)
		fromState := workflow.StateID(feat.Status)
		toState := workflow.StateID("in-progress")
		if !eng.CanTransition(fromState, toState) {
			return helpers.ErrorResult("workflow_error",
				fmt.Sprintf("cannot set to in-progress from status %q — feature must be in 'todo' or 'needs-edits'", feat.Status)), nil
		}

		// SESSION LOCK: acquire exclusive lock for this session.
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

func GetGateRequirements(store *storage.FeatureStorage, resolver *workflow.EngineResolver) ToolHandler {
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

		eng := resolver.Resolve(projectID)
		fromState := workflow.StateID(feat.Status)

		nextStates := eng.NextStates(fromState)
		if len(nextStates) == 0 {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is in terminal status **%s**.", featureID, feat.Status)), nil
		}

		if feat.Status == types.StatusInReview {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is **in-review**. Use `submit_review` (not advance_feature). "+
					"You MUST ask the user via `AskUserQuestion` first.", featureID)), nil
		}

		toState := nextStates[0]
		gate := eng.Gate(fromState, toState)

		if gate == nil {
			return helpers.TextResult(fmt.Sprintf(
				"Feature **%s** is **%s**. Next transition to **%s** is free — call advance_feature.",
				featureID, feat.Status, toState)), nil
		}

		msg := fmt.Sprintf("Feature **%s** is **%s**. Next transition to **%s** requires:\n\n"+
			"- Section: `## %s`\n"+
			"- Must include at least 1 file path(s)\n",
			featureID, feat.Status, toState, gate.RequiredSection)

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
