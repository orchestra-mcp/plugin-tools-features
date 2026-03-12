// Package internal contains the core registration logic for the tools.features
// plugin. The FeaturesPlugin struct wires all 101 tool handlers to the plugin
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

// RegisterTools registers all 101 tools on the given plugin builder.
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

	// --- Review tools (2) ---
	builder.RegisterTool("submit_review",
		"Submit a review decision (approved or needs-edits)",
		tools.SubmitReviewSchema(), tools.SubmitReview(s))
	builder.RegisterTool("get_pending_reviews",
		"Get all features pending review",
		tools.GetPendingReviewsSchema(), tools.GetPendingReviews(s))

	// --- Lock tools (1) ---
	builder.RegisterTool("unlock_feature",
		"Force-release a session lock on a feature (admin recovery)",
		tools.UnlockFeatureSchema(), tools.UnlockFeature(s))

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
	builder.RegisterTool("delete_plan",
		"Delete a plan from a project",
		tools.DeletePlanSchema(), tools.DeletePlan(s))

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

	// --- Test Case tools (2) ---
	builder.RegisterTool("create_test_case",
		"Create a test case linked to a feature (kind=testcase, docs gate auto-skipped)",
		tools.CreateTestCaseSchema(), tools.CreateTestCase(s))
	builder.RegisterTool("bulk_create_test_cases",
		"Create multiple test cases for a feature in one call",
		tools.BulkCreateTestCasesSchema(), tools.BulkCreateTestCases(s))

	// --- Person tools (5) ---
	builder.RegisterTool("create_person",
		"Create a person in the project registry with a role (developer/qa/reviewer/lead)",
		tools.CreatePersonSchema(), tools.CreatePerson(s))
	builder.RegisterTool("get_person",
		"Get a person's details from the project registry",
		tools.GetPersonSchema(), tools.GetPerson(s))
	builder.RegisterTool("list_persons",
		"List all persons in a project, optionally filtered by role or status",
		tools.ListPersonsSchema(), tools.ListPersons(s))
	builder.RegisterTool("update_person",
		"Update a person's name, email, role, or status",
		tools.UpdatePersonSchema(), tools.UpdatePerson(s))
	builder.RegisterTool("delete_person",
		"Delete a person from the project registry",
		tools.DeletePersonSchema(), tools.DeletePerson(s))

	// --- Assignment tools (5) ---
	builder.RegisterTool("bulk_assign_features",
		"Assign multiple features to one person in bulk",
		tools.BulkAssignFeaturesSchema(), tools.BulkAssignFeatures(s))
	builder.RegisterTool("create_assignment_rule",
		"Create an auto-assignment rule: when features of a kind are created, auto-assign to a person",
		tools.CreateAssignmentRuleSchema(), tools.CreateAssignmentRule(s))
	builder.RegisterTool("list_assignment_rules",
		"List all auto-assignment rules for a project",
		tools.ListAssignmentRulesSchema(), tools.ListAssignmentRules(s))
	builder.RegisterTool("delete_assignment_rule",
		"Delete an auto-assignment rule",
		tools.DeleteAssignmentRuleSchema(), tools.DeleteAssignmentRule(s))
	builder.RegisterTool("get_person_workload",
		"Get all features assigned to a person with status breakdown",
		tools.GetPersonWorkloadSchema(), tools.GetPersonWorkload(s))

	// --- Current User tools (3) ---
	builder.RegisterTool("set_current_user",
		"Link the current machine user to a person in a project (stored in ~/.orchestra/me.json)",
		tools.SetCurrentUserSchema(), tools.SetCurrentUser(s))
	builder.RegisterTool("get_current_user",
		"Get the current user's person data and workload summary",
		tools.GetCurrentUserSchema(), tools.GetCurrentUser(s))
	builder.RegisterTool("get_my_features",
		"List features assigned to the current user",
		tools.GetMyFeaturesSchema(), tools.GetMyFeatures(s))

	// --- Git tools (6) ---
	builder.RegisterTool("git_quick_commit",
		"Stage files and commit using the current user's identity (name/email from person profile)",
		tools.GitQuickCommitSchema(), tools.GitQuickCommit(s))
	builder.RegisterTool("git_push",
		"Push current branch to remote",
		tools.GitPushSchema(), tools.GitPush(s))
	builder.RegisterTool("git_pull",
		"Pull from remote with optional rebase",
		tools.GitPullSchema(), tools.GitPull(s))
	builder.RegisterTool("git_merge_branch",
		"Merge a branch into the current branch using person identity",
		tools.GitMergeBranchSchema(), tools.GitMergeBranch(s))
	builder.RegisterTool("git_status_summary",
		"Show compact git status: branch, ahead/behind, staged/unstaged/untracked counts",
		tools.GitStatusSummarySchema(), tools.GitStatusSummary(s))
	builder.RegisterTool("git_create_branch",
		"Create a new branch and switch to it",
		tools.GitCreateBranchSchema(), tools.GitCreateBranch(s))

	// --- Discovery Cycle tools (6) ---
	builder.RegisterTool("create_discovery_cycle",
		"Create a time-boxed discovery cycle (1-2 weeks) for hypothesis validation",
		tools.CreateDiscoveryCycleSchema(), tools.CreateDiscoveryCycle(s))
	builder.RegisterTool("get_discovery_cycle",
		"Get a discovery cycle's data and linked hypotheses/experiments",
		tools.GetDiscoveryCycleSchema(), tools.GetDiscoveryCycle(s))
	builder.RegisterTool("list_discovery_cycles",
		"List discovery cycles, optionally filtered by status",
		tools.ListDiscoveryCyclesSchema(), tools.ListDiscoveryCycles(s))
	builder.RegisterTool("update_discovery_cycle",
		"Update a discovery cycle's title, goal, or dates",
		tools.UpdateDiscoveryCycleSchema(), tools.UpdateDiscoveryCycle(s))
	builder.RegisterTool("complete_discovery_cycle",
		"Complete a discovery cycle with learnings and decision (continue/pivot/stop)",
		tools.CompleteDiscoveryCycleSchema(), tools.CompleteDiscoveryCycle(s))
	builder.RegisterTool("delete_discovery_cycle",
		"Delete a discovery cycle",
		tools.DeleteDiscoveryCycleSchema(), tools.DeleteDiscoveryCycle(s))

	// --- Hypothesis tools (7) ---
	builder.RegisterTool("create_hypothesis",
		"Create a user-problem hypothesis for discovery validation",
		tools.CreateHypothesisSchema(), tools.CreateHypothesis(s))
	builder.RegisterTool("get_hypothesis",
		"Get a hypothesis's data, experiments, and validation history",
		tools.GetHypothesisSchema(), tools.GetHypothesis(s))
	builder.RegisterTool("list_hypotheses",
		"List hypotheses, optionally filtered by status or cycle",
		tools.ListHypothesesSchema(), tools.ListHypotheses(s))
	builder.RegisterTool("update_hypothesis",
		"Update a hypothesis's title, problem, target user, or assumption",
		tools.UpdateHypothesisSchema(), tools.UpdateHypothesis(s))
	builder.RegisterTool("validate_hypothesis",
		"Mark a hypothesis as validated with evidence summary",
		tools.ValidateHypothesisSchema(), tools.ValidateHypothesis(s))
	builder.RegisterTool("invalidate_hypothesis",
		"Mark a hypothesis as invalidated with reason",
		tools.InvalidateHypothesisSchema(), tools.InvalidateHypothesis(s))
	builder.RegisterTool("refine_hypothesis",
		"Pivot: create a refined hypothesis from an existing one (marks original as refined)",
		tools.RefineHypothesisSchema(), tools.RefineHypothesis(s))

	// --- Experiment tools (9) ---
	builder.RegisterTool("create_experiment",
		"Create an experiment to test a hypothesis (interview, prototype, landing page, etc.)",
		tools.CreateExperimentSchema(), tools.CreateExperiment(s))
	builder.RegisterTool("get_experiment",
		"Get an experiment's data, signals, and outcome",
		tools.GetExperimentSchema(), tools.GetExperiment(s))
	builder.RegisterTool("list_experiments",
		"List experiments, optionally filtered by status, hypothesis, cycle, or kind",
		tools.ListExperimentsSchema(), tools.ListExperiments(s))
	builder.RegisterTool("update_experiment",
		"Update an experiment's method, success signal, or kill condition (draft only)",
		tools.UpdateExperimentSchema(), tools.UpdateExperiment(s))
	builder.RegisterTool("start_experiment",
		"Move an experiment from draft to running",
		tools.StartExperimentSchema(), tools.StartExperiment(s))
	builder.RegisterTool("record_signal",
		"Record a validation signal on a running experiment (user/behavior/market)",
		tools.RecordSignalSchema(), tools.RecordSignal(s))
	builder.RegisterTool("complete_experiment",
		"Complete an experiment with outcome summary",
		tools.CompleteExperimentSchema(), tools.CompleteExperiment(s))
	builder.RegisterTool("abandon_experiment",
		"Abandon an experiment, optionally marking kill condition as triggered",
		tools.AbandonExperimentSchema(), tools.AbandonExperiment(s))
	builder.RegisterTool("spawn_feature_from_experiment",
		"Create a feature from a completed experiment (bridges discovery into delivery)",
		tools.SpawnFeatureFromExperimentSchema(), tools.SpawnFeatureFromExperiment(s))

	// --- Discovery Review tools (3) ---
	builder.RegisterTool("create_discovery_review",
		"Create a weekly discovery review session for a cycle",
		tools.CreateDiscoveryReviewSchema(), tools.CreateDiscoveryReview(s))
	builder.RegisterTool("record_review_decisions",
		"Record decisions (continue/refine/pivot/stop) for hypotheses and experiments",
		tools.RecordReviewDecisionsSchema(), tools.RecordReviewDecisions(s))
	builder.RegisterTool("get_discovery_review",
		"Get a discovery review's decisions and insights",
		tools.GetDiscoveryReviewSchema(), tools.GetDiscoveryReview(s))

	// --- Discovery Mode tools (3) ---
	builder.RegisterTool("set_project_mode",
		"Set the project's operating mode (discovery/outcome/scale)",
		tools.SetProjectModeSchema(), tools.SetProjectMode(s))
	builder.RegisterTool("get_project_mode",
		"Get the project's current operating mode",
		tools.GetProjectModeSchema(), tools.GetProjectMode(s))
	builder.RegisterTool("check_transition_signals",
		"Analyze whether the project is ready to transition from discovery to outcome mode",
		tools.CheckTransitionSignalsSchema(), tools.CheckTransitionSignals(s))

	// --- Discovery Reporting tools (1) ---
	builder.RegisterTool("get_discovery_status",
		"Get comprehensive discovery dashboard with hypothesis/experiment counts, signals, and kill flags",
		tools.GetDiscoveryStatusSchema(), tools.GetDiscoveryStatus(s))
}
