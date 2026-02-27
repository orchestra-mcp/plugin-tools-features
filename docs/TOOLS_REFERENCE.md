# Tools Reference

The `tools.features` plugin provides 34 tools across 8 categories.

All tools accept arguments as a JSON object. Required fields are marked with **(required)**.

---

## Project Tools (4)

### `create_project`

Create a new project workspace.

| Param | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Project name |
| `description` | string | no | Project description |

The slug is auto-generated from the name (lowercased, non-alphanumeric replaced with hyphens).

### `list_projects`

List all projects. No parameters.

### `delete_project`

Delete a project and all its features.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

### `get_project_status`

Get project status with feature counts grouped by workflow status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

---

## Feature Tools (6)

### `create_feature`

Create a new feature in a project.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `title` | string | yes | Feature title |
| `description` | string | no | Feature description |
| `priority` | string | no | Priority: `P0`, `P1`, `P2` (default), `P3` |

Returns a generated feature ID in the format `FEAT-XXX` (three random uppercase letters).

### `get_feature`

Get a feature's metadata and body.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID (e.g., `FEAT-ABC`) |

### `update_feature`

Update a feature's title, description, or priority.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `title` | string | no | New title |
| `description` | string | no | New description |
| `priority` | string | no | New priority (`P0`-`P3`) |

### `list_features`

List all features in a project, optionally filtered by status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `status` | string | no | Filter by workflow status |

### `delete_feature`

Delete a feature from a project.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |

### `search_features`

Search features by title and description (case-insensitive substring match).

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `query` | string | yes | Search query |

---

## Workflow Tools (5)

### `advance_feature`

Advance a feature to the next valid workflow status. Takes the first valid transition (the "happy path"). See [WORKFLOW.md](WORKFLOW.md) for the state machine.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `evidence` | string | no | Evidence appended to the feature body |

### `reject_feature`

Reject a feature, setting it to `needs-edits`. Only valid from `in-review` status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `reason` | string | yes | Reason for rejection |

### `get_next_feature`

Get the next feature to work on based on priority and optional filters. Returns the highest-priority feature in the target status (default: `todo`).

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `status` | string | no | Filter by status (default: `todo`) |
| `assignee` | string | no | Filter by assignee |

### `set_current_feature`

Set a feature's status to `in-progress`. Only valid from `todo` or `needs-edits` status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |

### `get_workflow_status`

Get feature counts per workflow status for a project.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

---

## Review Tools (3)

### `request_review`

Request a review for a documented feature. Transitions from `documented` to `in-review`.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |

### `submit_review`

Submit a review decision. Transitions from `in-review` to either `done` (approved) or `needs-edits`.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `status` | string | yes | `approved` or `needs-edits` |
| `comment` | string | no | Review comment |

### `get_pending_reviews`

Get all features in `in-review` status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

---

## Dependency Tools (3)

### `add_dependency`

Add a dependency relationship: `feature_id` depends on `depends_on_id`. Also updates the `blocks` list on the target feature. Self-dependencies are rejected.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature that depends on another |
| `depends_on_id` | string | yes | Feature that is depended upon |

### `remove_dependency`

Remove a dependency relationship between two features.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature to remove dependency from |
| `depends_on_id` | string | yes | Feature to remove from dependencies |

### `get_dependency_graph`

Get the full dependency graph for a project. Lists all edges as "A depends on B".

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

---

## WIP Tools (3)

### `set_wip_limits`

Set the maximum number of features allowed in `in-progress` status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `max_in_progress` | number | yes | Maximum in-progress features (must be > 0) |

### `get_wip_limits`

Get the current WIP limits for a project.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

### `check_wip_limit`

Check whether the WIP limit would be exceeded. Reports current count vs. limit.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

---

## Reporting Tools (3)

### `get_progress`

Get project completion percentage and feature counts by status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

### `get_blocked_features`

Get features blocked by unfinished dependencies. A feature is blocked if any of its `depends_on` features are not in `done` status.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

### `get_review_queue`

Get features currently awaiting review (in `in-review` status).

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |

---

## Metadata Tools (7)

### `add_labels`

Add labels to a feature. Duplicates are ignored.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `labels` | string[] | yes | Labels to add |

### `remove_labels`

Remove labels from a feature.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `labels` | string[] | yes | Labels to remove |

### `assign_feature`

Assign a feature to a person.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `assignee` | string | yes | Assignee name or ID |

### `unassign_feature`

Remove the assignee from a feature.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |

### `set_estimate`

Set the size estimate for a feature.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `estimate` | string | yes | Size: `S`, `M`, `L`, or `XL` |

### `save_note`

Append a timestamped note to a feature's markdown body.

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
| `note` | string | yes | Note text to append |

### `list_notes`

Return the feature's markdown body (which contains all appended notes).

| Param | Type | Required | Description |
|---|---|---|---|
| `project_id` | string | yes | Project slug |
| `feature_id` | string | yes | Feature ID |
