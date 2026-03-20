package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/workflow"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreateWorkflowSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"name":          map[string]any{"type": "string", "description": "Workflow name (unique per project)"},
			"description":   map[string]any{"type": "string", "description": "Workflow description"},
			"initial_state": map[string]any{"type": "string", "description": "Initial state ID (default: todo)"},
			"states": map[string]any{
				"type":        "object",
				"description": "Map of state_id -> {label, terminal, active_work}",
			},
			"transitions": map[string]any{
				"type":        "array",
				"description": "Array of {from, to, gate} transition objects",
			},
			"gates": map[string]any{
				"type":        "object",
				"description": "Map of gate_id -> {label, required_section, file_patterns, docs_folder, skippable_for}",
			},
			"is_default": map[string]any{"type": "boolean", "description": "Set as the default workflow for this project (default: true)"},
		},
		"required": []any{"project_id", "name"},
	})
	return s
}

func GetWorkflowCRUDSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "Workflow ID (e.g. WFL-XXXX). If omitted, returns the project's default workflow."},
			"project_id":  map[string]any{"type": "string", "description": "Project slug (used when workflow_id is omitted)"},
		},
	})
	return s
}

func UpdateWorkflowSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id":   map[string]any{"type": "string", "description": "Workflow ID"},
			"name":          map[string]any{"type": "string", "description": "New name (optional)"},
			"description":   map[string]any{"type": "string", "description": "New description (optional)"},
			"initial_state": map[string]any{"type": "string", "description": "New initial state (optional)"},
			"states": map[string]any{
				"type":        "object",
				"description": "Replace all states (optional)",
			},
			"transitions": map[string]any{
				"type":        "array",
				"description": "Replace all transitions (optional)",
			},
			"gates": map[string]any{
				"type":        "object",
				"description": "Replace all gates (optional)",
			},
			"is_default": map[string]any{"type": "boolean", "description": "Set as default workflow (optional)"},
		},
		"required": []any{"workflow_id"},
	})
	return s
}

func DeleteWorkflowSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "Workflow ID to delete"},
		},
		"required": []any{"workflow_id"},
	})
	return s
}

func ListWorkflowsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Filter by project slug (optional, omit for all)"},
		},
	})
	return s
}

// ---------- Handlers ----------

// CreateWorkflowCRUD creates a new workflow definition in the database.
func CreateWorkflowCRUD(resolver *workflow.EngineResolver) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "name"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		name := helpers.GetString(req.Arguments, "name")
		desc := helpers.GetString(req.Arguments, "description")
		initialState := helpers.GetString(req.Arguments, "initial_state")
		if initialState == "" {
			initialState = "todo"
		}

		states := parseStates(req.Arguments)
		transitions := parseTransitions(req.Arguments)
		gates := parseGates(req.Arguments)

		// If no states/transitions provided, use the default workflow definition.
		if len(states) == 0 {
			def := workflow.DefaultDefinition()
			states, transitions, gates = defToRecords(def)
			initialState = string(def.InitialState)
		}

		// Validate the workflow structure.
		if err := validateWorkflowStructure(initialState, states, transitions, gates); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		isDefault := true
		if helpers.GetBool(req.Arguments, "is_default") {
			isDefault = true
		}

		id := helpers.NewWorkflowID()
		w := &globaldb.WorkflowRecord{
			ID:           id,
			ProjectID:    projectID,
			Name:         name,
			Description:  desc,
			InitialState: initialState,
			States:       states,
			Transitions:  transitions,
			Gates:        gates,
			IsDefault:    isDefault,
		}

		if err := globaldb.CreateWorkflowRecord(w); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				return helpers.ErrorResult("already_exists",
					fmt.Sprintf("workflow %q already exists for project %q", name, projectID)), nil
			}
			return helpers.ErrorResult("create_error", err.Error()), nil
		}

		// Invalidate engine cache for this project.
		if resolver != nil {
			resolver.Invalidate(projectID)
		}

		return helpers.TextResult(fmt.Sprintf("Created **%s**: %s\n\n%s",
			id, name, formatWorkflowSummary(w))), nil
	}
}

// GetWorkflowCRUD retrieves a workflow by ID or project default.
func GetWorkflowCRUD() ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		wfID := helpers.GetString(req.Arguments, "workflow_id")
		projectID := helpers.GetString(req.Arguments, "project_id")

		if wfID == "" && projectID == "" {
			return helpers.ErrorResult("validation_error", "provide workflow_id or project_id"), nil
		}

		var w *globaldb.WorkflowRecord
		var err error
		if wfID != "" {
			w, err = globaldb.GetWorkflowRecord(wfID)
		} else {
			w, err = globaldb.GetProjectWorkflow(projectID)
		}
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		return helpers.TextResult(formatWorkflowDetail(w)), nil
	}
}

// UpdateWorkflowCRUD updates an existing workflow.
func UpdateWorkflowCRUD(resolver *workflow.EngineResolver) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "workflow_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		wfID := helpers.GetString(req.Arguments, "workflow_id")
		w, err := globaldb.GetWorkflowRecord(wfID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if name := helpers.GetString(req.Arguments, "name"); name != "" {
			w.Name = name
		}
		if desc := helpers.GetString(req.Arguments, "description"); desc != "" {
			w.Description = desc
		}
		if is := helpers.GetString(req.Arguments, "initial_state"); is != "" {
			w.InitialState = is
		}

		if statesVal := req.Arguments.GetFields()["states"]; statesVal != nil {
			w.States = parseStates(req.Arguments)
		}
		if transVal := req.Arguments.GetFields()["transitions"]; transVal != nil {
			w.Transitions = parseTransitions(req.Arguments)
		}
		if gatesVal := req.Arguments.GetFields()["gates"]; gatesVal != nil {
			w.Gates = parseGates(req.Arguments)
		}

		if helpers.GetBool(req.Arguments, "is_default") {
			w.IsDefault = true
		}

		// Validate the updated workflow.
		if err := validateWorkflowStructure(w.InitialState, w.States, w.Transitions, w.Gates); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		if err := globaldb.SaveWorkflowRecord(w); err != nil {
			return helpers.ErrorResult("update_error", err.Error()), nil
		}

		// Invalidate engine cache for this project.
		if resolver != nil {
			resolver.Invalidate(w.ProjectID)
		}

		return helpers.TextResult(fmt.Sprintf("Updated **%s**\n\n%s", wfID, formatWorkflowSummary(w))), nil
	}
}

// DeleteWorkflowCRUD deletes a workflow.
func DeleteWorkflowCRUD(resolver *workflow.EngineResolver) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "workflow_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		wfID := helpers.GetString(req.Arguments, "workflow_id")

		// Look up the workflow to get project_id for cache invalidation.
		rec, _ := globaldb.GetWorkflowRecord(wfID)

		if err := globaldb.DeleteWorkflowRecord(wfID); err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Invalidate engine cache for the project.
		if resolver != nil && rec != nil {
			resolver.Invalidate(rec.ProjectID)
		}

		return helpers.TextResult(fmt.Sprintf("Deleted workflow **%s**", wfID)), nil
	}
}

// ListWorkflowsCRUD lists all workflows.
func ListWorkflowsCRUD() ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		projectID := helpers.GetString(req.Arguments, "project_id")
		workflows, err := globaldb.ListWorkflowRecords(projectID)
		if err != nil {
			return helpers.ErrorResult("list_error", err.Error()), nil
		}

		var sb strings.Builder
		title := "All Workflows"
		if projectID != "" {
			title = fmt.Sprintf("Workflows for %s", projectID)
		}
		sb.WriteString(fmt.Sprintf("## %s (%d)\n\n", title, len(workflows)))

		if len(workflows) == 0 {
			sb.WriteString("No workflows found. Use `create_workflow` to create one.\n")
			return helpers.TextResult(sb.String()), nil
		}

		sb.WriteString("| ID | Project | Name | States | Transitions | Default | Updated |\n")
		sb.WriteString("|---|---|---|---|---|---|---|\n")
		for _, w := range workflows {
			def := "no"
			if w.IsDefault {
				def = "**yes**"
			}
			updated := w.UpdatedAt
			if t, err := time.Parse(time.RFC3339, w.UpdatedAt); err == nil {
				updated = t.Format("2006-01-02")
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %d | %s | %s |\n",
				w.ID, w.ProjectID, w.Name, len(w.States), len(w.Transitions), def, updated))
		}

		return helpers.TextResult(sb.String()), nil
	}
}

// ---------- Helpers ----------

func parseStates(args *structpb.Struct) map[string]globaldb.WorkflowStateRec {
	result := make(map[string]globaldb.WorkflowStateRec)
	statesVal := args.GetFields()["states"]
	if statesVal == nil {
		return result
	}
	statesMap := statesVal.GetStructValue()
	if statesMap == nil {
		return result
	}
	for key, val := range statesMap.GetFields() {
		s := val.GetStructValue()
		if s == nil {
			continue
		}
		result[key] = globaldb.WorkflowStateRec{
			Label:      s.GetFields()["label"].GetStringValue(),
			Terminal:   s.GetFields()["terminal"].GetBoolValue(),
			ActiveWork: s.GetFields()["active_work"].GetBoolValue(),
		}
	}
	return result
}

func parseTransitions(args *structpb.Struct) []globaldb.WorkflowTransitionRec {
	var result []globaldb.WorkflowTransitionRec
	transVal := args.GetFields()["transitions"]
	if transVal == nil {
		return result
	}
	for _, item := range transVal.GetListValue().GetValues() {
		s := item.GetStructValue()
		if s == nil {
			continue
		}
		result = append(result, globaldb.WorkflowTransitionRec{
			From: s.GetFields()["from"].GetStringValue(),
			To:   s.GetFields()["to"].GetStringValue(),
			Gate: s.GetFields()["gate"].GetStringValue(),
		})
	}
	return result
}

func parseGates(args *structpb.Struct) map[string]globaldb.WorkflowGateRec {
	result := make(map[string]globaldb.WorkflowGateRec)
	gatesVal := args.GetFields()["gates"]
	if gatesVal == nil {
		return result
	}
	gatesMap := gatesVal.GetStructValue()
	if gatesMap == nil {
		return result
	}
	for key, val := range gatesMap.GetFields() {
		s := val.GetStructValue()
		if s == nil {
			continue
		}
		gate := globaldb.WorkflowGateRec{
			Label:           s.GetFields()["label"].GetStringValue(),
			RequiredSection: s.GetFields()["required_section"].GetStringValue(),
			DocsFolder:      s.GetFields()["docs_folder"].GetStringValue(),
		}
		if fp := s.GetFields()["file_patterns"]; fp != nil {
			for _, v := range fp.GetListValue().GetValues() {
				gate.FilePatterns = append(gate.FilePatterns, v.GetStringValue())
			}
		}
		if sf := s.GetFields()["skippable_for"]; sf != nil {
			for _, v := range sf.GetListValue().GetValues() {
				gate.SkippableFor = append(gate.SkippableFor, v.GetStringValue())
			}
		}
		result[key] = gate
	}
	return result
}

func defToRecords(def workflow.WorkflowDefinition) (
	map[string]globaldb.WorkflowStateRec,
	[]globaldb.WorkflowTransitionRec,
	map[string]globaldb.WorkflowGateRec,
) {
	states := make(map[string]globaldb.WorkflowStateRec)
	for id, s := range def.States {
		states[string(id)] = globaldb.WorkflowStateRec{
			Label:      s.Label,
			Terminal:   s.Terminal,
			ActiveWork: s.ActiveWork,
		}
	}
	var transitions []globaldb.WorkflowTransitionRec
	for _, t := range def.Transitions {
		transitions = append(transitions, globaldb.WorkflowTransitionRec{
			From: t.From,
			To:   t.To,
			Gate: t.Gate,
		})
	}
	gates := make(map[string]globaldb.WorkflowGateRec)
	for key, g := range def.Gates {
		gates[key] = globaldb.WorkflowGateRec{
			Label:           g.Label,
			RequiredSection: g.RequiredSection,
			FilePatterns:    g.FilePatterns,
			DocsFolder:      g.DocsFolder,
			SkippableFor:    g.SkippableFor,
		}
	}
	return states, transitions, gates
}

func validateWorkflowStructure(
	initialState string,
	states map[string]globaldb.WorkflowStateRec,
	transitions []globaldb.WorkflowTransitionRec,
	gates map[string]globaldb.WorkflowGateRec,
) error {
	if len(states) == 0 {
		return fmt.Errorf("workflow must have at least one state")
	}
	if _, ok := states[initialState]; !ok {
		return fmt.Errorf("initial_state %q not found in states", initialState)
	}

	// Check that all transitions reference valid states.
	for _, t := range transitions {
		if _, ok := states[t.From]; !ok {
			return fmt.Errorf("transition references unknown state %q (from)", t.From)
		}
		if _, ok := states[t.To]; !ok {
			return fmt.Errorf("transition references unknown state %q (to)", t.To)
		}
		if t.Gate != "" {
			if _, ok := gates[t.Gate]; !ok {
				return fmt.Errorf("transition references unknown gate %q", t.Gate)
			}
		}
	}

	// Check that at least one terminal state exists.
	hasTerminal := false
	for _, s := range states {
		if s.Terminal {
			hasTerminal = true
			break
		}
	}
	if !hasTerminal {
		return fmt.Errorf("workflow must have at least one terminal state")
	}

	return nil
}

func formatWorkflowSummary(w *globaldb.WorkflowRecord) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("- **Project:** %s\n", w.ProjectID))
	sb.WriteString(fmt.Sprintf("- **States:** %d\n", len(w.States)))
	sb.WriteString(fmt.Sprintf("- **Transitions:** %d\n", len(w.Transitions)))
	sb.WriteString(fmt.Sprintf("- **Gates:** %d\n", len(w.Gates)))
	if w.IsDefault {
		sb.WriteString("- **Default:** yes\n")
	}
	return sb.String()
}

func formatWorkflowDetail(w *globaldb.WorkflowRecord) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s — %s\n", w.ID, w.Name))
	sb.WriteString(fmt.Sprintf("- **Project:** %s\n", w.ProjectID))
	if w.Description != "" {
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", w.Description))
	}
	sb.WriteString(fmt.Sprintf("- **Initial State:** %s\n", w.InitialState))
	if w.IsDefault {
		sb.WriteString("- **Default:** yes\n")
	}
	sb.WriteString(fmt.Sprintf("- **Updated:** %s\n\n", w.UpdatedAt))

	// States table.
	sb.WriteString("#### States\n\n")
	sb.WriteString("| ID | Label | Terminal | Active Work |\n")
	sb.WriteString("|---|---|---|---|\n")
	for id, s := range w.States {
		sb.WriteString(fmt.Sprintf("| %s | %s | %v | %v |\n", id, s.Label, s.Terminal, s.ActiveWork))
	}

	// Transitions.
	sb.WriteString("\n#### Transitions\n\n")
	sb.WriteString("| From | To | Gate |\n")
	sb.WriteString("|---|---|---|\n")
	for _, t := range w.Transitions {
		gate := t.Gate
		if gate == "" {
			gate = "(free)"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", t.From, t.To, gate))
	}

	// Gates.
	if len(w.Gates) > 0 {
		sb.WriteString("\n#### Gates\n\n")
		sb.WriteString("| ID | Label | Required Section | Skippable For |\n")
		sb.WriteString("|---|---|---|---|\n")
		for id, g := range w.Gates {
			skip := "(none)"
			if len(g.SkippableFor) > 0 {
				skip = strings.Join(g.SkippableFor, ", ")
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | ## %s | %s |\n", id, g.Label, g.RequiredSection, skip))
		}
	}

	return sb.String()
}
