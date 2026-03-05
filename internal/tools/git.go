package tools

import (
	"context"
	"fmt"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal/git"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// ---------- Person identity resolution ----------

// resolveGitIdentity returns the git author name and email for a person.
// Priority: explicit person_id → current user from me.json → nil (use git config).
func resolveGitIdentity(ctx context.Context, store *storage.FeatureStorage, args *structpb.Struct) (name, email string) {
	personID := helpers.GetString(args, "person_id")
	projectID := helpers.GetString(args, "project_id")

	var person *types.PersonData

	if personID != "" && projectID != "" {
		p, _, _, err := store.ReadPerson(ctx, projectID, personID)
		if err == nil {
			person = p
		}
	}

	if person == nil {
		p, _, err := resolveCurrentUser(ctx, store, projectID)
		if err == nil {
			person = p
		}
	}

	if person == nil {
		return "", ""
	}

	name = person.Name
	email = person.GithubEmail
	if email == "" {
		email = person.Email
	}
	return name, email
}

// gitConfigArgs returns inline git config args for author identity.
func gitConfigArgs(name, email string, args ...string) []string {
	if name == "" || email == "" {
		return args
	}
	return append([]string{
		"-c", fmt.Sprintf("user.name=%s", name),
		"-c", fmt.Sprintf("user.email=%s", email),
	}, args...)
}

// ---------- Schemas ----------

func GitQuickCommitSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message":    map[string]any{"type": "string", "description": "Commit message"},
			"files":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Specific files to stage (optional)"},
			"all":        map[string]any{"type": "boolean", "description": "Stage all tracked modified files (git add -u)"},
			"person_id":  map[string]any{"type": "string", "description": "Person ID for commit author (optional, defaults to current user)"},
			"project_id": map[string]any{"type": "string", "description": "Project slug for person lookup (optional)"},
		},
		"required": []any{"message"},
	})
	return s
}

func GitPushSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"remote": map[string]any{"type": "string", "description": "Remote name (default: origin)"},
			"branch": map[string]any{"type": "string", "description": "Branch to push (default: current branch)"},
			"force":  map[string]any{"type": "boolean", "description": "Force push (use with caution)"},
		},
	})
	return s
}

func GitPullSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"remote": map[string]any{"type": "string", "description": "Remote name (default: origin)"},
			"branch": map[string]any{"type": "string", "description": "Branch to pull (default: current branch)"},
			"rebase": map[string]any{"type": "boolean", "description": "Use rebase instead of merge"},
		},
	})
	return s
}

func GitMergeBranchSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"branch":     map[string]any{"type": "string", "description": "Branch to merge into current"},
			"message":    map[string]any{"type": "string", "description": "Merge commit message (optional)"},
			"no_ff":      map[string]any{"type": "boolean", "description": "Always create a merge commit (--no-ff)"},
			"person_id":  map[string]any{"type": "string", "description": "Person ID for merge commit author (optional)"},
			"project_id": map[string]any{"type": "string", "description": "Project slug for person lookup (optional)"},
		},
		"required": []any{"branch"},
	})
	return s
}

func GitStatusSummarySchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	})
	return s
}

func GitCreateBranchSchema() *structpb.Struct {
	s, _ := structpb.NewStruct(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "New branch name"},
			"from": map[string]any{"type": "string", "description": "Starting point (branch, tag, or commit — default: HEAD)"},
		},
		"required": []any{"name"},
	})
	return s
}

// ---------- Handlers ----------

// GitQuickCommit stages files and creates a commit using the person's identity.
func GitQuickCommit(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "message"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		message := helpers.GetString(req.Arguments, "message")
		files := helpers.GetStringSlice(req.Arguments, "files")
		all := helpers.GetBool(req.Arguments, "all")

		// Stage files.
		if len(files) > 0 {
			addArgs := append([]string{"add"}, files...)
			if _, err := git.Run(ctx, "", nil, addArgs...); err != nil {
				return helpers.ErrorResult("git_error", err.Error()), nil
			}
		} else if all {
			if _, err := git.Run(ctx, "", nil, "add", "-u"); err != nil {
				return helpers.ErrorResult("git_error", err.Error()), nil
			}
		}

		// Resolve person identity.
		name, email := resolveGitIdentity(ctx, store, req.Arguments)

		// Commit with inline config for author identity.
		commitArgs := gitConfigArgs(name, email, "commit", "-m", message)
		output, err := git.Run(ctx, "", nil, commitArgs...)
		if err != nil {
			return helpers.ErrorResult("git_error", err.Error()), nil
		}

		// Get the commit hash.
		hash, _ := git.Run(ctx, "", nil, "rev-parse", "--short", "HEAD")

		var b strings.Builder
		fmt.Fprintf(&b, "Committed **%s**\n\n", hash)
		if name != "" {
			fmt.Fprintf(&b, "- **Author:** %s <%s>\n", name, email)
		}
		fmt.Fprintf(&b, "- **Message:** %s\n", message)
		fmt.Fprintf(&b, "\n```\n%s\n```\n", output)
		return helpers.TextResult(b.String()), nil
	}
}

// GitPush pushes the current branch to a remote.
func GitPush(_ *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		remote := helpers.GetStringOr(req.Arguments, "remote", "origin")
		branch := helpers.GetString(req.Arguments, "branch")
		force := helpers.GetBool(req.Arguments, "force")

		args := []string{"push", remote}
		if branch != "" {
			args = append(args, branch)
		}
		if force {
			args = append(args, "--force")
		}

		output, err := git.Run(ctx, "", nil, args...)
		if err != nil {
			return helpers.ErrorResult("git_error", err.Error()), nil
		}

		if output == "" {
			output = "Push completed successfully."
		}
		return helpers.TextResult(fmt.Sprintf("```\n%s\n```", output)), nil
	}
}

// GitPull pulls from a remote with optional rebase.
func GitPull(_ *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		remote := helpers.GetStringOr(req.Arguments, "remote", "origin")
		branch := helpers.GetString(req.Arguments, "branch")
		rebase := helpers.GetBool(req.Arguments, "rebase")

		args := []string{"pull", remote}
		if branch != "" {
			args = append(args, branch)
		}
		if rebase {
			args = append(args, "--rebase")
		}

		output, err := git.Run(ctx, "", nil, args...)
		if err != nil {
			return helpers.ErrorResult("git_error", err.Error()), nil
		}

		return helpers.TextResult(fmt.Sprintf("```\n%s\n```", output)), nil
	}
}

// GitMergeBranch merges a branch into the current branch.
func GitMergeBranch(store *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "branch"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		branch := helpers.GetString(req.Arguments, "branch")
		message := helpers.GetString(req.Arguments, "message")
		noFF := helpers.GetBool(req.Arguments, "no_ff")

		name, email := resolveGitIdentity(ctx, store, req.Arguments)

		mergeArgs := []string{"merge", branch}
		if noFF {
			mergeArgs = append(mergeArgs, "--no-ff")
		}
		if message != "" {
			mergeArgs = append(mergeArgs, "-m", message)
		}

		// Use inline config for merge commit author.
		fullArgs := gitConfigArgs(name, email, mergeArgs...)
		output, err := git.Run(ctx, "", nil, fullArgs...)
		if err != nil {
			return helpers.ErrorResult("git_error", err.Error()), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Merged **%s** into current branch\n\n", branch)
		if name != "" {
			fmt.Fprintf(&b, "- **Author:** %s <%s>\n", name, email)
		}
		fmt.Fprintf(&b, "\n```\n%s\n```\n", output)
		return helpers.TextResult(b.String()), nil
	}
}

// GitStatusSummary returns a compact git status overview.
func GitStatusSummary(_ *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, _ *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		// Current branch.
		branch, err := git.Run(ctx, "", nil, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return helpers.ErrorResult("git_error", err.Error()), nil
		}

		// Porcelain status for file counts.
		status, _ := git.Run(ctx, "", nil, "status", "--porcelain")
		var staged, unstaged, untracked int
		for _, line := range strings.Split(status, "\n") {
			if len(line) < 2 {
				continue
			}
			x, y := line[0], line[1]
			if x == '?' {
				untracked++
			} else {
				if x != ' ' && x != '?' {
					staged++
				}
				if y != ' ' && y != '?' {
					unstaged++
				}
			}
		}

		// Ahead/behind remote.
		var ahead, behind string
		upstreamStatus, err := git.Run(ctx, "", nil, "rev-list", "--left-right", "--count", "@{u}...HEAD")
		if err == nil {
			parts := strings.Fields(upstreamStatus)
			if len(parts) == 2 {
				behind = parts[0]
				ahead = parts[1]
			}
		}

		// Last commit.
		lastCommit, _ := git.Run(ctx, "", nil, "log", "-1", "--format=%h %s (%ar)")

		var b strings.Builder
		fmt.Fprintf(&b, "## Git Status\n\n")
		fmt.Fprintf(&b, "- **Branch:** %s\n", branch)
		if ahead != "" || behind != "" {
			fmt.Fprintf(&b, "- **Ahead:** %s, **Behind:** %s\n", ahead, behind)
		}
		fmt.Fprintf(&b, "- **Staged:** %d, **Unstaged:** %d, **Untracked:** %d\n", staged, unstaged, untracked)
		if lastCommit != "" {
			fmt.Fprintf(&b, "- **Last commit:** %s\n", lastCommit)
		}
		return helpers.TextResult(b.String()), nil
	}
}

// GitCreateBranch creates a new branch and switches to it.
func GitCreateBranch(_ *storage.FeatureStorage) ToolHandler {
	return func(ctx context.Context, req *pluginv1.ToolRequest) (*pluginv1.ToolResponse, error) {
		if err := helpers.ValidateRequired(req.Arguments, "name"); err != nil {
			return helpers.ErrorResult("validation_error", err.Error()), nil
		}

		name := helpers.GetString(req.Arguments, "name")
		from := helpers.GetString(req.Arguments, "from")

		args := []string{"checkout", "-b", name}
		if from != "" {
			args = append(args, from)
		}

		output, err := git.Run(ctx, "", nil, args...)
		if err != nil {
			return helpers.ErrorResult("git_error", err.Error()), nil
		}

		md := fmt.Sprintf("Created and switched to branch **%s**\n\n```\n%s\n```", name, output)
		return helpers.TextResult(md), nil
	}
}
