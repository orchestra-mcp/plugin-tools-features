# Contributing to plugin-tools-features

## Prerequisites

- Go 1.23+
- `gofmt`, `go vet`

## Development Setup

```bash
git clone https://github.com/orchestra-mcp/plugin-tools-features.git
cd plugin-tools-features
go mod download
go build ./cmd/...
```

## Running Locally

```bash
go build -o tools-features ./cmd/
./tools-features --orchestrator-addr=localhost:50100 --certs-dir=~/.orchestra/certs
```

The plugin connects to the orchestrator as a client and also starts its own QUIC server for incoming requests.

## Running Tests

```bash
go test ./...
```

Tests use an in-memory storage backend (`storage.MemoryStorage`) so they run without disk I/O or a running orchestrator.

## Code Organization

```
plugin-tools-features/
  cmd/main.go                    # Entry point
  internal/
    features.go                  # RegisterTools: wires all 34 tools to the plugin builder
    storage/
      client.go                  # FeatureStorage: QUIC-backed storage client
      memory.go                  # MemoryStorage: in-memory backend for tests
    tools/
      project.go                 # create_project, list_projects, delete_project, get_project_status
      feature.go                 # create_feature, get_feature, update_feature, list_features, delete_feature, search_features
      workflow.go                # advance_feature, reject_feature, get_next_feature, set_current_feature, get_workflow_status
      review.go                  # request_review, submit_review, get_pending_reviews
      dependency.go              # add_dependency, remove_dependency, get_dependency_graph
      wip.go                     # set_wip_limits, get_wip_limits, check_wip_limit
      reporting.go               # get_progress, get_blocked_features, get_review_queue
      metadata.go                # add_labels, remove_labels, assign_feature, unassign_feature, set_estimate, save_note, list_notes
```

## Adding a New Tool

1. Create the schema function and handler function in the appropriate file under `internal/tools/`.
2. Register the tool in `internal/features.go` via `builder.RegisterTool(...)`.
3. Add a test case in `internal/features_test.go`.
4. Update `docs/TOOLS_REFERENCE.md`.

## Code Style

- Run `gofmt` on all files.
- Run `go vet ./...` before committing.
- All exported functions and types must have doc comments.
- Each tool file should have a `// ---------- Schemas ----------` section followed by a `// ---------- Handlers ----------` section.
- Use `helpers.ValidateRequired` for argument validation.
- Use `helpers.TextResult` / `helpers.ErrorResult` for building responses.
- Never return a Go error from a tool handler for expected failures (validation errors, not-found). Use `helpers.ErrorResult` instead. Reserve Go errors for unexpected infrastructure failures.

## Pull Request Process

1. Fork the repository and create a feature branch from `main`.
2. Write or update tests for your changes. All tools should have test coverage.
3. Run `go test ./...` and `go vet ./...`.
4. Update `docs/TOOLS_REFERENCE.md` and `docs/WORKFLOW.md` if applicable.

## Related Repositories

- [orchestra-mcp/proto](https://github.com/orchestra-mcp/proto) -- Protobuf schema
- [orchestra-mcp/sdk-go](https://github.com/orchestra-mcp/sdk-go) -- Go Plugin SDK
- [orchestra-mcp/orchestrator](https://github.com/orchestra-mcp/orchestrator) -- Central hub
- [orchestra-mcp/plugin-storage-markdown](https://github.com/orchestra-mcp/plugin-storage-markdown) -- Storage backend
- [orchestra-mcp](https://github.com/orchestra-mcp) -- Organization home
