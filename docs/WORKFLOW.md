# Workflow

## State Machine

The feature-driven workflow has 11 states. Features progress through these states in a cyclical pattern until a human approves them.

```
                    +----------+
                    | backlog  |
                    +----+-----+
                         |
                         v
                    +----+-----+
                    |   todo   |
                    +----+-----+
                         |
                         v
              +---------+-----------+
              |    in-progress      |<-------+
              +---------+-----------+        |
                        |                    |
                        v                    |
             +----------+-----------+        |
             | ready-for-testing    |        |
             +----------+-----------+        |
                        |                    |
                        v                    |
             +----------+-----------+        |
             |    in-testing        +--------+  (test failure)
             +----------+-----------+
                        |
                        v
             +----------+-----------+
             |  ready-for-docs      |
             +----------+-----------+
                        |
                        v
             +----------+-----------+
             |     in-docs          |
             +----------+-----------+
                        |
                        v
             +----------+-----------+
             |    documented        |
             +----------+-----------+
                        |
                        v
             +----------+-----------+
             |    in-review         +------->+
             +----------+-----------+        |
                        |                    |
                        v                    v
                 +------+------+    +--------+-------+
                 |    done     |    |  needs-edits   |
                 +-------------+    +--------+-------+
                                             |
                                             | (back to in-progress)
                                             v
                                    +--------+-------+
                                    |  in-progress   |
                                    +----------------+
```

## Valid Transitions

| From | To |
|---|---|
| `backlog` | `todo` |
| `todo` | `in-progress` |
| `in-progress` | `ready-for-testing` |
| `ready-for-testing` | `in-testing` |
| `in-testing` | `ready-for-docs`, `in-progress` |
| `ready-for-docs` | `in-docs` |
| `in-docs` | `documented` |
| `documented` | `in-review` |
| `in-review` | `done`, `needs-edits` |
| `needs-edits` | `in-progress` |
| `done` | (terminal -- no outgoing transitions) |

## How Tools Use the Workflow

### `advance_feature`

Takes the **first** valid transition from the current status. This is the "happy path" -- always moves forward. For statuses with multiple outgoing transitions (like `in-testing` or `in-review`), `advance_feature` picks the first target:

- `in-testing` advances to `ready-for-docs` (not back to `in-progress`)
- `in-review` advances to `done` (not `needs-edits`)

To take the alternate path, use the specific tools instead:
- From `in-review`, use `reject_feature` to go to `needs-edits`
- From `in-testing`, there is no dedicated "fail test" tool; use `set_current_feature` if the feature needs rework

### `set_current_feature`

Directly transitions to `in-progress`. Valid from `todo` or `needs-edits`.

### `reject_feature`

Directly transitions to `needs-edits`. Only valid from `in-review`.

### `request_review`

Directly transitions to `in-review`. Only valid from `documented`.

### `submit_review`

From `in-review`, transitions to `done` (if approved) or `needs-edits` (if rejected).

## Cyclical Delivery

The workflow supports cyclical delivery: a feature can loop through the cycle multiple times:

```
in-progress -> ... -> in-review -> needs-edits -> in-progress -> ... -> in-review -> done
```

Each cycle, evidence and review comments are appended to the feature's markdown body, creating a full audit trail. A feature is only `done` when a human reviewer approves it.

## Evidence Trail

When `advance_feature` is called with an `evidence` parameter, it appends to the feature body:

```markdown
---
**in-progress -> ready-for-testing**: All unit tests pass, 95% coverage
```

Review submissions also append:

```markdown
---
**Review (approved)**: LGTM, ship it
```
