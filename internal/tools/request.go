package tools

import (
	"context"
	"fmt"
	"sort"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreateRequestSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":  map[string]any{"type": "string", "description": "Project slug"},
			"title":       map[string]any{"type": "string", "description": "Request title"},
			"description": map[string]any{"type": "string", "description": "Request description"},
			"kind":        map[string]any{"type": "string", "description": "Request kind", "enum": []any{"feature", "hotfix", "bug"}},
			"priority":    map[string]any{"type": "string", "description": "Priority (P0-P3)", "enum": []any{"P0", "P1", "P2", "P3"}},
		},
		"required": []any{"project_id", "title", "kind"},
	})
	return s
}

func ListRequestsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)"},
			"kind":       map[string]any{"type": "string", "description": "Filter by kind (optional)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

func GetRequestSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"request_id": map[string]any{"type": "string", "description": "Request ID (e.g., REQ-ABC)"},
		},
		"required": []any{"project_id", "request_id"},
	})
	return s
}

func ConvertRequestSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"request_id": map[string]any{"type": "string", "description": "Request ID (e.g., REQ-ABC)"},
		},
		"required": []any{"project_id", "request_id"},
	})
	return s
}

func DismissRequestSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"request_id": map[string]any{"type": "string", "description": "Request ID (e.g., REQ-ABC)"},
			"reason":     map[string]any{"type": "string", "description": "Reason for dismissal"},
		},
		"required": []any{"project_id", "request_id", "reason"},
	})
	return s
}

func GetNextRequestSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"kind":       map[string]any{"type": "string", "description": "Filter by kind (optional)"},
		},
		"required": []any{"project_id"},
	})
	return s
}

// ---------- Handlers ----------

// CreateRequest creates a new request in the project queue.
func CreateRequest(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "title", "kind"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		title := helpers.GetString(req.Arguments, "title")
		description := helpers.GetString(req.Arguments, "description")
		kind := helpers.GetString(req.Arguments, "kind")
		priority := helpers.GetStringOr(req.Arguments, "priority", "P2")

		// Validate kind.
		if err := helpers.ValidateOneOf(kind, "feature", "hotfix", "bug"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Validate priority.
		if err := helpers.ValidateOneOf(priority, "P0", "P1", "P2", "P3"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		requestID := helpers.NewRequestID()
		now := helpers.NowISO()

		r := &types.RequestData{
			ID:          requestID,
			ProjectID:   projectID,
			Title:       title,
			Description: description,
			Kind:        kind,
			Status:      types.RequestPending,
			Priority:    priority,
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		body := fmt.Sprintf("# %s\n\n%s\n", title, description)

		_, err = store.WriteRequest(ctx, projectID, requestID, r, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created **%s**: %s\n\n%s", requestID, title, helpers.FormatRequestMD(r))
		return helpers.TextResult(md), nil
	}
}

// ListRequests returns all requests in a project, optionally filtered by status and/or kind.
func ListRequests(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")
		kindFilter := helpers.GetString(req.Arguments, "kind")

		requests, err := store.ListRequests(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		if statusFilter != "" {
			var filtered []*types.RequestData
			for _, r := range requests {
				if string(r.Status) == statusFilter {
					filtered = append(filtered, r)
				}
			}
			requests = filtered
		}

		if kindFilter != "" {
			var filtered []*types.RequestData
			for _, r := range requests {
				if r.Kind == kindFilter {
					filtered = append(filtered, r)
				}
			}
			requests = filtered
		}

		if requests == nil {
			requests = []*types.RequestData{}
		}

		header := "Requests"
		if statusFilter != "" && kindFilter != "" {
			header = fmt.Sprintf("Requests (%s, %s)", statusFilter, kindFilter)
		} else if statusFilter != "" {
			header = fmt.Sprintf("Requests (%s)", statusFilter)
		} else if kindFilter != "" {
			header = fmt.Sprintf("Requests (%s)", kindFilter)
		}
		return helpers.TextResult(helpers.FormatRequestListMD(requests, header)), nil
	}
}

// GetRequest returns a request's data and body.
func GetRequest(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "request_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		requestID := helpers.GetString(req.Arguments, "request_id")

		r, body, _, err := store.ReadRequest(ctx, projectID, requestID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		md := helpers.FormatRequestMD(r) + "---\n\n" + body
		return helpers.TextResult(md), nil
	}
}

// ConvertRequest converts a pending or picked-up request into a feature.
func ConvertRequest(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "request_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		requestID := helpers.GetString(req.Arguments, "request_id")

		// Read the request.
		r, _, version, err := store.ReadRequest(ctx, projectID, requestID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Verify status allows conversion.
		if r.Status != types.RequestPending && r.Status != types.RequestPickedUp {
			return helpers.ErrorResult("invalid_status",
				fmt.Sprintf("request %s has status %q, must be %q or %q to convert",
					requestID, r.Status, types.RequestPending, types.RequestPickedUp)), nil
		}

		// Create the feature.
		featureID := helpers.NewFeatureID()
		now := helpers.NowISO()

		feat := &types.FeatureData{
			ID:          featureID,
			ProjectID:   projectID,
			Title:       r.Title,
			Description: r.Description,
			Status:      types.StatusTodo,
			Priority:    r.Priority,
			Kind:        types.FeatureKind(r.Kind),
			Labels:      []string{fmt.Sprintf("request:%s", requestID)},
			Version:     0,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		featureBody := fmt.Sprintf("# %s\n\n%s\n\nConverted from request %s\n", r.Title, r.Description, requestID)

		_, err = store.WriteFeature(ctx, projectID, featureID, feat, featureBody, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Update the request status.
		r.Status = types.RequestConverted
		r.ConvertedTo = featureID
		r.UpdatedAt = now

		requestBody := fmt.Sprintf("# %s\n\n%s\n", r.Title, r.Description)

		_, err = store.WriteRequest(ctx, projectID, requestID, r, requestBody, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Converted **%s** → **%s**\n\n%s", requestID, featureID, helpers.FormatFeatureMD(feat))
		return helpers.TextResult(md), nil
	}
}

// DismissRequest marks a pending or picked-up request as dismissed with a reason.
func DismissRequest(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "request_id", "reason"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		requestID := helpers.GetString(req.Arguments, "request_id")
		reason := helpers.GetString(req.Arguments, "reason")

		// Read the request.
		r, body, version, err := store.ReadRequest(ctx, projectID, requestID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Verify status allows dismissal.
		if r.Status != types.RequestPending && r.Status != types.RequestPickedUp {
			return helpers.ErrorResult("invalid_status",
				fmt.Sprintf("request %s has status %q, must be %q or %q to dismiss",
					requestID, r.Status, types.RequestPending, types.RequestPickedUp)), nil
		}

		// Update status and append reason to body.
		r.Status = types.RequestDismissed
		r.UpdatedAt = helpers.NowISO()

		body += fmt.Sprintf("\n\n---\n**Dismissed:** %s\n", reason)

		_, err = store.WriteRequest(ctx, projectID, requestID, r, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("Dismissed **%s**: %s", requestID, reason)), nil
	}
}

// GetNextRequest returns the highest-priority pending request, optionally filtered by kind.
func GetNextRequest(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		kindFilter := helpers.GetString(req.Arguments, "kind")

		requests, err := store.ListRequests(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Filter to pending requests only.
		var pending []*types.RequestData
		for _, r := range requests {
			if r.Status != types.RequestPending {
				continue
			}
			if kindFilter != "" && r.Kind != kindFilter {
				continue
			}
			pending = append(pending, r)
		}

		if len(pending) == 0 {
			return helpers.TextResult("No pending requests found."), nil
		}

		// Sort by priority (P0 highest → P3 lowest).
		sort.Slice(pending, func(i, j int) bool {
			return priorityRank(pending[i].Priority) < priorityRank(pending[j].Priority)
		})

		return helpers.TextResult(helpers.FormatRequestMD(pending[0])), nil
	}
}

// priorityRank returns a numeric rank for sorting priorities.
// Lower values represent higher priority.
func priorityRank(p string) int {
	switch p {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	default:
		return 3
	}
}
