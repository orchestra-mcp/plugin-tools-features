package tools

import (
	"context"
	"fmt"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Schemas ----------

func CreatePersonSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":   map[string]any{"type": "string", "description": "Project slug"},
			"name":         map[string]any{"type": "string", "description": "Person's full name"},
			"email":        map[string]any{"type": "string", "description": "Person's email address (optional)"},
			"role":         map[string]any{"type": "string", "description": "Role within the project", "enum": []any{"developer", "qa", "reviewer", "lead"}},
			"labels":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional labels"},
			"bio":          map[string]any{"type": "string", "description": "Short bio or description (optional)"},
			"github_email": map[string]any{"type": "string", "description": "GitHub email for git commits (optional, falls back to email)"},
			"integrations": map[string]any{"type": "object", "description": "Key-value integration data (e.g. jira_email, slack_id, timezone, github_username)"},
		},
		"required": []any{"project_id", "name", "role"},
	})
	return s
}

func GetPersonSchema() *structpb.Struct {
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

func ListPersonsSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"role":       map[string]any{"type": "string", "description": "Filter by role (optional)", "enum": []any{"developer", "qa", "reviewer", "lead"}},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)", "enum": []any{"active", "inactive"}},
		},
		"required": []any{"project_id"},
	})
	return s
}

func UpdatePersonSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id":   map[string]any{"type": "string", "description": "Project slug"},
			"person_id":    map[string]any{"type": "string", "description": "Person ID (PERS-XXX)"},
			"name":         map[string]any{"type": "string", "description": "New name (optional)"},
			"email":        map[string]any{"type": "string", "description": "New email (optional)"},
			"role":         map[string]any{"type": "string", "description": "New role (optional)", "enum": []any{"developer", "qa", "reviewer", "lead"}},
			"status":       map[string]any{"type": "string", "description": "New status (optional)", "enum": []any{"active", "inactive"}},
			"bio":          map[string]any{"type": "string", "description": "Short bio or description (optional)"},
			"github_email": map[string]any{"type": "string", "description": "GitHub email for git commits (optional)"},
			"integrations": map[string]any{"type": "object", "description": "Key-value integration data (e.g. jira_email, slack_id, timezone)"},
		},
		"required": []any{"project_id", "person_id"},
	})
	return s
}

func DeletePersonSchema() *structpb.Struct {
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

// CreatePerson creates a new person in the project registry.
func CreatePerson(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "name", "role"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		name := helpers.GetString(req.Arguments, "name")
		email := helpers.GetString(req.Arguments, "email")
		role := helpers.GetString(req.Arguments, "role")
		labels := helpers.GetStringSlice(req.Arguments, "labels")

		if err := helpers.ValidateOneOf(role, types.ValidRoles...); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		// Verify the project exists.
		_, _, err := store.ReadProject(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("project %q not found", projectID)), nil
		}

		bio := helpers.GetString(req.Arguments, "bio")
		githubEmail := helpers.GetString(req.Arguments, "github_email")
		integrations := helpers.GetStringMap(req.Arguments, "integrations")

		personID := helpers.NewPersonID()
		now := helpers.NowISO()

		person := &types.PersonData{
			ID:           personID,
			ProjectID:    projectID,
			Name:         name,
			Email:        email,
			Role:         types.PersonRole(role),
			Status:       types.PersonActive,
			Labels:       labels,
			Bio:          bio,
			GithubEmail:  githubEmail,
			Integrations: integrations,
			Version:      0,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		body := fmt.Sprintf("# %s\n\nRole: %s\n", name, role)

		_, err = store.WritePerson(ctx, projectID, personID, person, body, 0)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created person **%s**: %s (%s)\n\n%s", personID, name, role, helpers.FormatPersonMD(person))
		return helpers.TextResult(md), nil
	}
}

// GetPerson returns a person's details.
func GetPerson(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")

		person, _, _, err := store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		return helpers.TextResult(helpers.FormatPersonMD(person)), nil
	}
}

// ListPersons lists all persons in a project, optionally filtered by role or status.
func ListPersons(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		roleFilter := helpers.GetString(req.Arguments, "role")
		statusFilter := helpers.GetString(req.Arguments, "status")

		persons, err := store.ListPersons(ctx, projectID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		// Apply filters.
		var filtered []*types.PersonData
		for _, p := range persons {
			if roleFilter != "" && string(p.Role) != roleFilter {
				continue
			}
			if statusFilter != "" && string(p.Status) != statusFilter {
				continue
			}
			filtered = append(filtered, p)
		}

		return helpers.TextResult(helpers.FormatPersonListMD(filtered, "Persons")), nil
	}
}

// UpdatePerson updates a person's name, email, role, or status.
func UpdatePerson(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")

		person, body, version, err := store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		if name := helpers.GetString(req.Arguments, "name"); name != "" {
			person.Name = name
		}
		if email := helpers.GetString(req.Arguments, "email"); email != "" {
			person.Email = email
		}
		if role := helpers.GetString(req.Arguments, "role"); role != "" {
			if err := helpers.ValidateOneOf(role, types.ValidRoles...); err != nil {
				return helpers.ErrorResult("validation_error", err.Error()), nil
			}
			person.Role = types.PersonRole(role)
		}
		if status := helpers.GetString(req.Arguments, "status"); status != "" {
			if err := helpers.ValidateOneOf(status, types.ValidPersonStatuses...); err != nil {
				return helpers.ErrorResult("validation_error", err.Error()), nil
			}
			person.Status = types.PersonStatus(status)
		}
		if bio := helpers.GetString(req.Arguments, "bio"); bio != "" {
			person.Bio = bio
		}
		if githubEmail := helpers.GetString(req.Arguments, "github_email"); githubEmail != "" {
			person.GithubEmail = githubEmail
		}
		if integrations := helpers.GetStringMap(req.Arguments, "integrations"); integrations != nil {
			if person.Integrations == nil {
				person.Integrations = make(map[string]string)
			}
			for k, v := range integrations {
				person.Integrations[k] = v
			}
		}
		person.UpdatedAt = helpers.NowISO()

		_, err = store.WritePerson(ctx, projectID, personID, person, body, version)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Updated **%s**\n\n%s", personID, helpers.FormatPersonMD(person))
		return helpers.TextResult(md), nil
	}
}

// DeletePerson removes a person from the project registry. Warns if the person
// has active feature assignments.
func DeletePerson(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")

		// Verify person exists.
		_, _, _, err := store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", err.Error()), nil
		}

		// Check for active assignments.
		features, _ := store.ListFeatures(ctx, projectID)
		var activeCount int
		for _, f := range features {
			if f.Assignee == personID && f.Status != types.StatusDone {
				activeCount++
			}
		}

		err = store.DeletePerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		md := fmt.Sprintf("Deleted **%s**", personID)
		if activeCount > 0 {
			md += fmt.Sprintf("\n\n**Warning:** %d feature(s) are still assigned to this person. Consider reassigning them.", activeCount)
		}
		return helpers.TextResult(md), nil
	}
}
