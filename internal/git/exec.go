package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run executes a git command in the specified directory and returns stdout.
// If dir is empty, it uses ORCHESTRA_WORKSPACE env var.
// The env map injects additional environment variables (e.g. GIT_AUTHOR_NAME).
func Run(ctx context.Context, dir string, env map[string]string, args ...string) (string, error) {
	if dir == "" {
		dir = os.Getenv("ORCHESTRA_WORKSPACE")
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), mapToEnv(env)...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), errMsg)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func mapToEnv(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, k+"="+v)
	}
	return result
}
