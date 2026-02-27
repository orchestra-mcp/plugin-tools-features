# Orchestra Tools Features Plugin

Feature-driven workflow engine providing 34 tools for project and feature management.

## Install

```bash
go get github.com/orchestra-mcp/plugin-tools-features
```

## Usage

```bash
# Build
go build -o bin/tools-features ./cmd/

# Run (started automatically by the orchestrator)
bin/tools-features --orchestrator-addr localhost:9100
```

## Tools (34)

Organized into 8 categories:

| Category | Tools |
|----------|-------|
| **Project** | `create_project`, `get_project`, `list_projects`, `delete_project`, `get_project_status` |
| **Feature** | `create_feature`, `get_feature`, `list_features`, `update_feature`, `delete_feature`, `search_features` |
| **Workflow** | `advance_feature`, `set_current_feature`, `get_next_feature`, `get_workflow_status` |
| **Review** | `request_review`, `submit_review`, `reject_feature`, `get_review_queue`, `get_pending_reviews` |
| **Dependencies** | `add_dependency`, `remove_dependency`, `get_dependency_graph`, `get_blocked_features` |
| **WIP Limits** | `set_wip_limits`, `get_wip_limits`, `check_wip_limit` |
| **Reporting** | `get_progress`, `save_note`, `list_notes` |
| **Metadata** | `add_labels`, `remove_labels`, `assign_feature`, `unassign_feature`, `set_estimate` |

## Workflow States

Features move through an 11-state lifecycle:

```
backlog -> todo -> in-progress -> ready-for-review -> in-review ->
ready-for-testing -> in-testing -> ready-for-docs -> in-docs -> documented -> done
```

## Related Packages

| Package | Description |
|---------|-------------|
| [sdk-go](https://github.com/orchestra-mcp/sdk-go) | Plugin SDK this plugin is built on |
| [orchestrator](https://github.com/orchestra-mcp/orchestrator) | Central hub that loads this plugin |
| [plugin-storage-markdown](https://github.com/orchestra-mcp/plugin-storage-markdown) | Storage backend for features |

## License

[MIT](LICENSE)
