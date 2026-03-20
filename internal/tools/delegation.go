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

func DelegateFeatureSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Feature ID to delegate from"},
			"to_person":  map[string]any{"type": "string", "description": "Person ID to delegate to (e.g., PERS-ABC)"},
			"question":   map[string]any{"type": "string", "description": "The question or decision needed from the delegated person"},
			"context":    map[string]any{"type": "string", "description": "Additional context for the delegated person"},
		},
		"required": []any{"project_id", "feature_id", "to_person", "question"},
	})
	return s
}

func RespondDelegationSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"delegation_id": map[string]any{"type": "string", "description": "Delegation ID (e.g., DEL-ABC)"},
			"response":      map[string]any{"type": "string", "description": "The answer or decision"},
		},
		"required": []any{"project_id", "delegation_id", "response"},
	})
	return s
}

func GetDelegationSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":    map[string]any{"type": "string", "description": "Project slug"},
			"delegation_id": map[string]any{"type": "string", "description": "Delegation ID (e.g., DEL-ABC)"},
		},
		"required": []any{"project_id", "delegation_id"},
	})
	return s
}

func ListDelegationsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"feature_id": map[string]any{"type": "string", "description": "Filter by feature ID (optional)"},
			"person_id":  map[string]any{"type": "string", "description": "Filter by person ID (from or to, optional)"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)", "enum": []any{"pending", "answered", "dismissed"}},
			"limit":      map[string]any{"type": "number", "description": "Max results (default 50, max 200)"},
			"offset":     map[string]any{"type": "number", "description": "Skip first N results (default 0)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func GetPendingDelegationsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"person_id":  map[string]any{"type": "string", "description": "Person ID to get pending delegations for (e.g., PERS-ABC)"},
		},
		"required": []any{"project_id", "person_id"},
	})
	return s
}

// ---------- Handlers ----------

// DelegateFeature creates a delegation request, marking the feature as blocked.
func DelegateFeature(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "feature_id", "to_person", "question"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureID := helpers.GetString(req.Arguments, "feature_id")
		toPerson := helpers.GetString(req.Arguments, "to_person")
		question := helpers.GetString(req.Arguments, "question")
		ctxText := helpers.GetString(req.Arguments, "context")

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		// Read the feature.
		feat, featBody, featVersion, err := store.ReadFeature(ctx, projectID, featureID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("feature %q not found", featureID)), nil
		}

		// Feature must be in-progress to delegate.
		if feat.Status != types.StatusInProgress {
			return helpers.ErrorResult("invalid_status",
				fmt.Sprintf("feature %s has status %q, must be %q to delegate",
					featureID, feat.Status, types.StatusInProgress)), nil
		}

		// Verify the target person exists.
		_, _, _, err = store.ReadPerson(ctx, projectID, toPerson)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("person %q not found", toPerson)), nil
		}

		// Determine from_person: current user → feature assignee → "unknown".
		fromPerson := ""
		if me, _, err := resolveCurrentUser(ctx, store, projectID); err == nil && me != nil {
			fromPerson = me.ID
		}
		if fromPerson == "" {
			fromPerson = feat.Assignee
		}
		if fromPerson == "" {
			fromPerson = "unknown"
		}

		delegationID := helpers.NewDelegationID()
		now := helpers.NowISO()

		d := &types.DelegationData{
			ID:         delegationID,
			ProjectID:  projectID,
			FeatureID:  featureID,
			FromPerson: fromPerson,
			ToPerson:   toPerson,
			Question:   question,
			Context:    ctxText,
			Status:     types.DelegationPending,
			Version:    0,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		body := fmt.Sprintf("# Delegation %s\n\n**Question:** %s\n", delegationID, question)
		if ctxText != "" {
			body += fmt.Sprintf("\n**Context:** %s\n", ctxText)
		}

		_, err = store.WriteDelegation(ctx, projectID, delegationID, d, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Add delegation label to the feature to block it.
		label := fmt.Sprintf("delegation:%s", delegationID)
		feat.Labels = append(feat.Labels, label)
		feat.UpdatedAt = now

		// Append delegation info to feature body as audit trail.
		featBody += fmt.Sprintf("\n\n---\n**Delegated** (%s): %s → %s\n> %s\n", now, fromPerson, toPerson, question)

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, featBody, featVersion)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created delegation **%s**\n\n%s\nFeature **%s** is now blocked until **%s** responds.\n",
			delegationID, helpers.FormatDelegationMD(d), featureID, toPerson)
		ui := helpers.DelegationDetailUI(d)
		return helpers.RichResult(md, ui), nil
	}
}

// RespondDelegation submits an answer and unblocks the feature.
func RespondDelegation(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "delegation_id", "response"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		delegationID := helpers.GetString(req.Arguments, "delegation_id")
		response := helpers.GetString(req.Arguments, "response")

		// Read the delegation.
		d, delBody, delVersion, err := store.ReadDelegation(ctx, projectID, delegationID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("delegation %q not found", delegationID)), nil
		}

		// Must be pending.
		if d.Status != types.DelegationPending {
			return helpers.ErrorResult("invalid_status",
				fmt.Sprintf("delegation %s has status %q, must be %q to respond",
					delegationID, d.Status, types.DelegationPending)), nil
		}

		now := helpers.NowISO()
		d.Response = response
		d.Status = types.DelegationAnswered
		d.RespondedAt = now
		d.UpdatedAt = now

		delBody += fmt.Sprintf("\n\n---\n**Response** (%s):\n%s\n", now, response)

		_, err = store.WriteDelegation(ctx, projectID, delegationID, d, delBody, delVersion)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Remove delegation label from the feature.
		feat, featBody, featVersion, err := store.ReadFeature(ctx, projectID, d.FeatureID)
		if err == nil {
			label := fmt.Sprintf("delegation:%s", delegationID)
			var newLabels []string
			for _, l := range feat.Labels {
				if l != label {
					newLabels = append(newLabels, l)
				}
			}
			feat.Labels = newLabels
			feat.UpdatedAt = now

			// Append response to feature body.
			featBody += fmt.Sprintf("\n\n---\n**Delegation %s answered** (%s) by %s:\n> %s\n",
				delegationID, now, d.ToPerson, response)

			_, _ = store.WriteFeature(ctx, projectID, d.FeatureID, feat, featBody, featVersion)
		}

		md := fmt.Sprintf("Responded to delegation **%s**\n\nFeature **%s** is now unblocked.\n\n**Response:** %s\n",
			delegationID, d.FeatureID, response)
		return helpers.TextResult(md), nil
	}
}

// GetDelegation returns a delegation's data and body.
func GetDelegation(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "delegation_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		delegationID := helpers.GetString(req.Arguments, "delegation_id")

		d, body, _, err := store.ReadDelegation(ctx, projectID, delegationID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		md := helpers.FormatDelegationMD(d) + "---\n\n" + body
		ui := helpers.DelegationDetailUI(d)
		return helpers.RichResult(md, ui), nil
	}
}

// ListDelegations returns delegations for a project, optionally filtered.
func ListDelegations(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		featureFilter := helpers.GetString(req.Arguments, "feature_id")
		personFilter := helpers.GetString(req.Arguments, "person_id")
		statusFilter := helpers.GetString(req.Arguments, "status")

		delegations, err := store.ListDelegations(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if featureFilter != "" {
			var filtered []*types.DelegationData
			for _, d := range delegations {
				if d.FeatureID == featureFilter {
					filtered = append(filtered, d)
				}
			}
			delegations = filtered
		}

		if personFilter != "" {
			var filtered []*types.DelegationData
			for _, d := range delegations {
				if d.FromPerson == personFilter || d.ToPerson == personFilter {
					filtered = append(filtered, d)
				}
			}
			delegations = filtered
		}

		if statusFilter != "" {
			var filtered []*types.DelegationData
			for _, d := range delegations {
				if string(d.Status) == statusFilter {
					filtered = append(filtered, d)
				}
			}
			delegations = filtered
		}

		if delegations == nil {
			delegations = []*types.DelegationData{}
		}

		total := len(delegations)
		pg := helpers.ParsePagination(req.Arguments)
		delegations = helpers.PaginateSlice(delegations, pg)

		header := "Delegations"
		if statusFilter != "" {
			header = fmt.Sprintf("Delegations (%s)", statusFilter)
		}
		md := helpers.FormatDelegationListMD(delegations, header)
		md += fmt.Sprintf("\n*Showing %d-%d of %d total*\n", pg.Offset+1, pg.Offset+len(delegations), total)
		ui := helpers.DelegationListUI(delegations, pg, total)
		return helpers.RichResult(md, ui), nil
	}
}

// GetPendingDelegations returns delegations pending response for a specific person.
func GetPendingDelegations(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")

		delegations, err := store.ListDelegations(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var pending []*types.DelegationData
		for _, d := range delegations {
			if d.ToPerson == personID && d.Status == types.DelegationPending {
				pending = append(pending, d)
			}
		}

		if pending == nil {
			pending = []*types.DelegationData{}
		}

		pg := helpers.PaginationParams{Limit: len(pending), Offset: 0}
		md := helpers.FormatDelegationListMD(pending, fmt.Sprintf("Pending Delegations for %s", personID))
		ui := helpers.DelegationListUI(pending, pg, len(pending))
		return helpers.RichResult(md, ui), nil
	}
}

// HasPendingDelegation checks if a feature has any pending delegation labels.
// Returns the first pending delegation ID found, or empty string if none.
func HasPendingDelegation(ctx context.Context, store *storage.FeatureStorage, projectID string, feat *types.FeatureData) (string, string) {
	for _, label := range feat.Labels {
		if strings.HasPrefix(label, "delegation:") {
			delID := strings.TrimPrefix(label, "delegation:")
			d, _, _, err := store.ReadDelegation(ctx, projectID, delID)
			if err == nil && d.Status == types.DelegationPending {
				return delID, d.ToPerson
			}
		}
	}
	return "", ""
}
