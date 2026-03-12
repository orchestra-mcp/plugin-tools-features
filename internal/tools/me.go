package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/globaldb"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// resolveCurrentUser looks up the current user from globaldb, resolves the person
// ID for the given project, and fetches the full PersonData from storage.
// Person profiles are stored as markdown in .projects/ for git team sync.
func resolveCurrentUser(ctx context.Context, store *storage.FeatureStorage, projectID string) (*types.PersonData, string, error) {
	// Determine which project to use.
	pid := projectID
	if pid == "" {
		pid = globaldb.GetDefaultProject()
	}
	if pid == "" {
		return nil, "", fmt.Errorf("no project specified and no default_project set — run set_current_user first")
	}

	personID := globaldb.GetCurrentUser(pid)
	if personID == "" {
		return nil, "", fmt.Errorf("no current user set for project %q — run set_current_user first", pid)
	}

	person, _, _, err := store.ReadPerson(ctx, pid, personID)
	if err != nil {
		return nil, "", fmt.Errorf("person %s not found in project %s: %w", personID, pid, err)
	}

	return person, pid, nil
}

// ---------- Schemas ----------

func SetCurrentUserSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug"},
			"person_id":  map[string]any{"type": "string", "description": "Person ID (PERS-XXX) to set as current user"},
		},
		"required": []any{"project_id", "person_id"},
	})
	return s
}

func GetCurrentUserSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug (optional, uses default project if omitted)"},
		},
	})
	return s
}

func GetMyFeaturesSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string", "description": "Project slug (optional, uses default project if omitted)"},
			"status":     map[string]any{"type": "string", "description": "Filter by status (optional)"},
		},
	})
	return s
}

// ---------- Handlers ----------

// SetCurrentUser links the current machine user to a person in a project.
// The mapping is stored in globaldb. Person profiles remain as markdown in
// .projects/<project>/persons/ for git team sync.
func SetCurrentUser(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "project_id", "person_id"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		projectID := helpers.GetString(req.Arguments, "project_id")
		personID := helpers.GetString(req.Arguments, "person_id")

		// Validate person exists (reads from storage — person profile lives as markdown).
		person, _, _, err := store.ReadPerson(ctx, projectID, personID)
		if err != nil {
			return helpers.ErrorResult("not_found", fmt.Sprintf("person %s not found in project %s", personID, projectID)), nil
		}

		// Store the mapping in globaldb (not in a file that could conflict on git).
		if err := globaldb.SetCurrentUser(projectID, personID); err != nil {
			return helpers.ErrorResult("config_error", err.Error()), nil
		}

		// Set default project if not yet configured.
		if globaldb.GetDefaultProject() == "" {
			globaldb.SetDefaultProject(projectID)
		}

		md := fmt.Sprintf("Set current user for project **%s** to **%s** (%s, %s)\n\n%s",
			projectID, person.Name, personID, person.Role, helpers.FormatPersonMD(person))
		return helpers.TextResult(md), nil
	}
}

// GetCurrentUser returns the current user's person data for a project.
func GetCurrentUser(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		projectID := helpers.GetString(req.Arguments, "project_id")

		person, resolvedProject, err := resolveCurrentUser(ctx, store, projectID)
		if err != nil {
			return helpers.ErrorResult("not_configured", err.Error()), nil
		}

		// Get workload summary.
		features, _ := store.ListFeatures(ctx, resolvedProject)
		counts := make(map[string]int)
		var total int
		for _, f := range features {
			if f.Assignee == person.ID {
				counts[string(f.Status)]++
				total++
			}
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Current User (%s)\n\n", resolvedProject)
		b.WriteString(helpers.FormatPersonMD(person))
		if total > 0 {
			b.WriteString("\n### Workload\n\n")
			b.WriteString(helpers.FormatStatusCountsMD(counts, total))
		} else {
			b.WriteString("\nNo features assigned.\n")
		}
		return helpers.TextResult(b.String()), nil
	}
}

// GetMyFeatures lists features assigned to the current user.
func GetMyFeatures(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		projectID := helpers.GetString(req.Arguments, "project_id")
		statusFilter := helpers.GetString(req.Arguments, "status")
		sessionID := req.GetSessionId()

		person, resolvedProject, err := resolveCurrentUser(ctx, store, projectID)
		if err != nil {
			return helpers.ErrorResult("not_configured", err.Error()), nil
		}

		features, err := store.ListFeatures(ctx, resolvedProject)
		if err != nil {
			return helpers.ErrorResult("storage_error", err.Error()), nil
		}

		var mine []*types.FeatureData
		for _, f := range features {
			if f.Assignee != person.ID {
				continue
			}
			if statusFilter != "" && string(f.Status) != statusFilter {
				continue
			}
			mine = append(mine, f)
		}

		header := fmt.Sprintf("My Features — %s (%s)", person.Name, resolvedProject)
		return helpers.TextResult(helpers.FormatFeatureListMDWithLocks(mine, header, resolvedProject, sessionID)), nil
	}
}
