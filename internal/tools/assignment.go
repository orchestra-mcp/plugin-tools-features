package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func BulkAssignFeaturesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"feature_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Array of feature IDs to assign"},
			"person_id":   map[string]any{"type": "string", "description": "Person ID (PERS-XXX) to assign features to"},
		},
		"required": []any{"project_id", "feature_ids", "person_id"},
	})
	return s
}

func CreateAssignmentRuleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"kind":       map[string]any{"type": "string", "description": "Feature kind to match", "enum": []any{"feature", "bug", "hotfix", "chore", "testcase"}},
			"person_id":  map[string]any{"type": "string", "description": "Person ID (PERS-XXX) to auto-assign to"},
		},
		"required": []any{"project_id", "kind", "person_id"},
	})
	return s
}

func ListAssignmentRulesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func DeleteAssignmentRuleSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"rule_id":    map[string]any{"type": "string", "description": "Assignment rule ID (RULE-XXX)"},
		},
		"required": []any{"project_id", "rule_id"},
	})
	return s
}

func GetPersonWorkloadSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"person_id":  map[string]any{"type": "string", "description": "Person ID (PERS-XXX)"},
		},
		"required": []any{"project_id", "person_id"},
	})
	return s
}

// ---------- Handlers ----------

// BulkAssignFeatures assigns multiple features to one person.
func BulkAssignFeatures(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")
		featureIDs := helpers.GetStringSlice(req.Arguments, "feature_ids")

		if len(featureIDs) == 0 {
			return helpers.ErrorResult("validation_error", "feature_ids must not be empty"), nil
		}

		// Verify person exists.
		_, _, _, err := store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("person %q not found", personID)), nil
		}

		var assigned []string
		var failed []string
		for _, fid := range featureIDs {
			feat, body, version, err := store.ReadFeature(ctx, projectID, fid)
			if err != nil {
				failed = append(failed, fid)
				continue
			}
			feat.Assignee = personID
			feat.UpdatedAt = helpers.NowISO()
			_, err = store.WriteFeature(ctx, projectID, fid, feat, body, version)
			if err != nil {
				failed = append(failed, fid)
				continue
			}
			assigned = append(assigned, fid)
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Assigned **%d** feature(s) to **%s**", len(assigned), personID)
		if len(assigned) > 0 {
			fmt.Fprintf(&b, ": %s", strings.Join(assigned, ", "))
		}
		if len(failed) > 0 {
			fmt.Fprintf(&b, "\n\n**Failed:** %s", strings.Join(failed, ", "))
		}
		return helpers.TextResult(b.String()), nil
	}
}

// CreateAssignmentRule creates an auto-assignment rule for a feature kind. When
// features of the specified kind are created, they are automatically assigned
// to the given person. Only one rule per kind is allowed.
func CreateAssignmentRule(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "kind", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		kind := helpers.GetString(req.Arguments, "kind")
		personID := helpers.GetString(req.Arguments, "person_id")

		if err := helpers.ValidateOneOf(kind, types.ValidKinds...); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Verify project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		// Verify person exists.
		_, _, _, err = store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("person %q not found", personID)), nil
		}

		// Check for existing rule with same kind.
		existing, _ := store.ListAssignmentRules(ctx, projectID)
		for _, r := range existing {
			if r.Kind == kind {
				return helpers.ErrorResult("validation_error",
					fmt.Sprintf("assignment rule for kind %q already exists (%s). Delete it first with delete_assignment_rule.", kind, r.ID)), nil
			}
		}

		ruleID := helpers.NewRuleID()
		now := helpers.NowISO()

		rule := &types.AssignmentRuleData{
			ID:        ruleID,
			ProjectID: projectID,
			Kind:      kind,
			PersonID:  personID,
			Version:   0,
			CreatedAt: now,
			UpdatedAt: now,
		}

		body := fmt.Sprintf("# Assignment Rule\n\nAuto-assign %s features to %s\n", kind, personID)

		_, err = store.WriteAssignmentRule(ctx, projectID, ruleID, rule, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created assignment rule **%s**: auto-assign kind=%s to %s\n\n%s",
			ruleID, kind, personID, helpers.FormatAssignmentRuleMD(rule))
		return helpers.TextResult(md), nil
	}
}

// ListAssignmentRules lists all auto-assignment rules for a project.
func ListAssignmentRules(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")

		rules, err := store.ListAssignmentRules(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(helpers.FormatAssignmentRuleListMD(rules, "Assignment Rules")), nil
	}
}

// DeleteAssignmentRule deletes an auto-assignment rule.
func DeleteAssignmentRule(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "rule_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		ruleID := helpers.GetString(req.Arguments, "rule_id")

		// Verify rule exists.
		_, _, _, err := store.ReadAssignmentRule(ctx, projectID, ruleID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		err = store.DeleteAssignmentRule(ctx, projectID, ruleID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Deleted assignment rule **%s**", ruleID)), nil
	}
}

// GetPersonWorkload shows all features assigned to a person with status breakdown.
func GetPersonWorkload(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")

		// Verify person exists.
		person, _, _, err := store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		features, err := store.ListFeatures(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Filter features assigned to this person.
		var assigned []*types.FeatureData
		statusCounts := make(map[string]int)
		for _, f := range features {
			if f.Assignee == personID {
				assigned = append(assigned, f)
				statusCounts[string(f.Status)]++
			}
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Workload for %s (%s, %s)\n\n", person.Name, personID, person.Role)

		if len(assigned) == 0 {
			fmt.Fprintf(&b, "No features assigned.\n")
			return helpers.TextResult(b.String()), nil
		}

		// Status breakdown.
		fmt.Fprintf(&b, "### Status Breakdown\n\n")
		fmt.Fprintf(&b, "| Status | Count |\n")
		fmt.Fprintf(&b, "|--------|-------|\n")
		for status, count := range statusCounts {
			fmt.Fprintf(&b, "| %s | %d |\n", status, count)
		}
		fmt.Fprintf(&b, "| **Total** | **%d** |\n", len(assigned))

		// Feature list.
		fmt.Fprintf(&b, "\n")
		fmt.Fprintf(&b, "%s", helpers.FormatFeatureListMD(assigned, "Assigned Features"))

		return helpers.TextResult(b.String()), nil
	}
}
