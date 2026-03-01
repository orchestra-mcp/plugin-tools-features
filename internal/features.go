// Package internal contains the core registration logic for the tools.features
// plugin. The FeaturesPlugin struct wires all 49 tool handlers to the plugin
// builder with their schemas and descriptions.
package internal

import (
	"github.com/orchestra-mcp/sdk-go/plugin"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/plugin-tools-features/internal/tools"
)

// FeaturesPlugin holds the shared dependencies for all tool handlers.
type FeaturesPlugin struct {
	Storage *storage.FeatureStorage
}

// RegisterTools registers all 49 tools on the given plugin builder.
func (fp *FeaturesPlugin) RegisterTools(builder *plugin.PluginBuilder) {
	s := fp.Storage

	// --- Project tools (4) ---
	builder.RegisterTool("create_project",
		"Create a new project workspace",
		tools.CreateProjectSchema(), tools.CreateProject(s))
	builder.RegisterTool("list_projects",
		"List all projects",
		tools.ListProjectsSchema(), tools.ListProjects(s))
	builder.RegisterTool("delete_project",
		"Delete a project and all its features",
		tools.DeleteProjectSchema(), tools.DeleteProject(s))
	builder.RegisterTool("get_project_status",
		"Get project status with feature counts by status",
		tools.GetProjectStatusSchema(), tools.GetProjectStatus(s))

	// --- Feature tools (6) ---
	builder.RegisterTool("create_feature",
		"Create a new feature in a project",
		tools.CreateFeatureSchema(), tools.CreateFeature(s))
	builder.RegisterTool("get_feature",
		"Get a feature's data and body",
		tools.GetFeatureSchema(), tools.GetFeature(s))
	builder.RegisterTool("update_feature",
		"Update a feature's title, description, or priority",
		tools.UpdateFeatureSchema(), tools.UpdateFeature(s))
	builder.RegisterTool("list_features",
		"List all features in a project, optionally filtered by status",
		tools.ListFeaturesSchema(), tools.ListFeatures(s))
	builder.RegisterTool("delete_feature",
		"Delete a feature from a project",
		tools.DeleteFeatureSchema(), tools.DeleteFeature(s))
	builder.RegisterTool("search_features",
		"Search features by title and description",
		tools.SearchFeaturesSchema(), tools.SearchFeatures(s))

	// --- Workflow tools (5) ---
	builder.RegisterTool("advance_feature",
		"Advance a feature to the next workflow status",
		tools.AdvanceFeatureSchema(), tools.AdvanceFeature(s))
	builder.RegisterTool("reject_feature",
		"Reject a feature, setting it to needs-edits",
		tools.RejectFeatureSchema(), tools.RejectFeature(s))
	builder.RegisterTool("get_next_feature",
		"Get the next feature to work on based on priority and filters",
		tools.GetNextFeatureSchema(), tools.GetNextFeature(s))
	builder.RegisterTool("set_current_feature",
		"Set a feature's status to in-progress",
		tools.SetCurrentFeatureSchema(), tools.SetCurrentFeature(s))
	builder.RegisterTool("get_workflow_status",
		"Get feature counts per workflow status",
		tools.GetWorkflowStatusSchema(), tools.GetWorkflowStatus(s))
	builder.RegisterTool("get_gate_requirements",
		"Get the gate requirements for the next transition of a feature. Shows what evidence is needed before advance_feature will succeed.",
		tools.GetGateRequirementsSchema(), tools.GetGateRequirements(s))

	// --- Review tools (3) ---
	builder.RegisterTool("request_review",
		"Request a review for a documented feature",
		tools.RequestReviewSchema(), tools.RequestReview(s))
	builder.RegisterTool("submit_review",
		"Submit a review decision (approved or needs-edits)",
		tools.SubmitReviewSchema(), tools.SubmitReview(s))
	builder.RegisterTool("get_pending_reviews",
		"Get all features pending review",
		tools.GetPendingReviewsSchema(), tools.GetPendingReviews(s))

	// --- Dependency tools (3) ---
	builder.RegisterTool("add_dependency",
		"Add a dependency between two features",
		tools.AddDependencySchema(), tools.AddDependency(s))
	builder.RegisterTool("remove_dependency",
		"Remove a dependency between two features",
		tools.RemoveDependencySchema(), tools.RemoveDependency(s))
	builder.RegisterTool("get_dependency_graph",
		"Get the full dependency graph for a project",
		tools.GetDependencyGraphSchema(), tools.GetDependencyGraph(s))

	// --- WIP tools (3) ---
	builder.RegisterTool("set_wip_limits",
		"Set the maximum number of in-progress features",
		tools.SetWIPLimitsSchema(), tools.SetWIPLimits(s))
	builder.RegisterTool("get_wip_limits",
		"Get the current WIP limits for a project",
		tools.GetWIPLimitsSchema(), tools.GetWIPLimits(s))
	builder.RegisterTool("check_wip_limit",
		"Check if the WIP limit would be exceeded",
		tools.CheckWIPLimitSchema(), tools.CheckWIPLimit(s))

	// --- Reporting tools (3) ---
	builder.RegisterTool("get_progress",
		"Get project completion percentage and status breakdown",
		tools.GetProgressSchema(), tools.GetProgress(s))
	builder.RegisterTool("get_blocked_features",
		"Get features blocked by unfinished dependencies",
		tools.GetBlockedFeaturesSchema(), tools.GetBlockedFeatures(s))
	builder.RegisterTool("get_review_queue",
		"Get features currently awaiting review",
		tools.GetReviewQueueSchema(), tools.GetReviewQueue(s))

	// --- Metadata tools (7) ---
	builder.RegisterTool("add_labels",
		"Add labels to a feature",
		tools.AddLabelsSchema(), tools.AddLabels(s))
	builder.RegisterTool("remove_labels",
		"Remove labels from a feature",
		tools.RemoveLabelsSchema(), tools.RemoveLabels(s))
	builder.RegisterTool("assign_feature",
		"Assign a feature to a person",
		tools.AssignFeatureSchema(), tools.AssignFeature(s))
	builder.RegisterTool("unassign_feature",
		"Remove the assignee from a feature",
		tools.UnassignFeatureSchema(), tools.UnassignFeature(s))
	builder.RegisterTool("set_estimate",
		"Set the size estimate for a feature (S/M/L/XL)",
		tools.SetEstimateSchema(), tools.SetEstimate(s))
	builder.RegisterTool("save_note",
		"Append a note to a feature's body",
		tools.SaveNoteSchema(), tools.SaveNote(s))
	builder.RegisterTool("list_notes",
		"List all notes in a feature's body",
		tools.ListNotesSchema(), tools.ListNotes(s))

	// --- Plan tools (7) ---
	builder.RegisterTool("create_plan",
		"Create a new plan for breaking down a large task into features",
		tools.CreatePlanSchema(), tools.CreatePlan(s))
	builder.RegisterTool("get_plan",
		"Get a plan's data, body, and linked features",
		tools.GetPlanSchema(), tools.GetPlan(s))
	builder.RegisterTool("list_plans",
		"List all plans in a project, optionally filtered by status",
		tools.ListPlansSchema(), tools.ListPlans(s))
	builder.RegisterTool("update_plan",
		"Update a plan's title or description",
		tools.UpdatePlanSchema(), tools.UpdatePlan(s))
	builder.RegisterTool("approve_plan",
		"Approve a draft plan for implementation",
		tools.ApprovePlanSchema(), tools.ApprovePlan(s))
	builder.RegisterTool("breakdown_plan",
		"Break down an approved plan into features with dependencies",
		tools.BreakdownPlanSchema(), tools.BreakdownPlan(s))
	builder.RegisterTool("complete_plan",
		"Mark a plan as completed after all features are done",
		tools.CompletePlanSchema(), tools.CompletePlan(s))

	// --- Request tools (6) ---
	builder.RegisterTool("create_request",
		"Save a user request to the queue (for processing after current work)",
		tools.CreateRequestSchema(), tools.CreateRequest(s))
	builder.RegisterTool("list_requests",
		"List queued user requests, optionally filtered by status or kind",
		tools.ListRequestsSchema(), tools.ListRequests(s))
	builder.RegisterTool("get_request",
		"Get a request's data and body",
		tools.GetRequestSchema(), tools.GetRequest(s))
	builder.RegisterTool("convert_request",
		"Convert a pending request into a feature",
		tools.ConvertRequestSchema(), tools.ConvertRequest(s))
	builder.RegisterTool("dismiss_request",
		"Dismiss a request with a reason",
		tools.DismissRequestSchema(), tools.DismissRequest(s))
	builder.RegisterTool("get_next_request",
		"Get the highest-priority pending request",
		tools.GetNextRequestSchema(), tools.GetNextRequest(s))

	// --- Bug tools (1) ---
	builder.RegisterTool("create_bug_report",
		"Report a bug, optionally linked to the feature that caused it",
		tools.CreateBugReportSchema(), tools.CreateBugReport(s))
}
