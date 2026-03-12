// Package storage provides an abstraction over the orchestrator's storage
// protocol for reading and writing feature and project data. The StorageClient
// interface allows swapping a real QUIC-based client for an in-memory fake
// during testing.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/helpers"
	"github.com/orchestra-mcp/sdk-go/types"
	"google.golang.org/protobuf/types/known/structpb"
)

// StorageClient is the interface that tool handlers use to communicate with the
// orchestrator. In production this is backed by a QUIC OrchestratorClient. In
// tests it is replaced by InMemoryStorage.
type StorageClient interface {
	Send(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
}

// FeatureStorage provides high-level operations for reading and writing feature
// and project data. It translates domain types into storage protocol messages.
type FeatureStorage struct {
	client StorageClient
}

// NewFeatureStorage creates a new FeatureStorage backed by the given client.
func NewFeatureStorage(client StorageClient) *FeatureStorage {
	return &FeatureStorage{client: client}
}

// ---------- Project operations ----------

// ReadProject loads a project by slug from storage.
func (fs *FeatureStorage) ReadProject(ctx context.Context, projectSlug string) (*types.ProjectData, int64, error) {
	path := filepath.Join(projectSlug, helpers.ConfigFile)
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, 0, fmt.Errorf("read project %s: %w", projectSlug, err)
	}

	proj, err := metadataToProject(resp.Metadata)
	if err != nil {
		return nil, 0, fmt.Errorf("parse project %s: %w", projectSlug, err)
	}
	return proj, resp.Version, nil
}

// WriteProject persists a project to storage.
func (fs *FeatureStorage) WriteProject(ctx context.Context, data *types.ProjectData, expectedVersion int64) (int64, error) {
	meta, err := projectToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode project: %w", err)
	}
	path := filepath.Join(data.Slug, helpers.ConfigFile)
	return fs.storageWrite(ctx, path, meta, nil, expectedVersion)
}

// ListProjects returns all projects found in storage.
func (fs *FeatureStorage) ListProjects(ctx context.Context) ([]*types.ProjectData, error) {
	entries, err := fs.storageList(ctx, "", "project.json")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	var projects []*types.ProjectData
	for _, entry := range entries {
		// entry.Path is relative like "my-project/project.json"
		parts := strings.SplitN(entry.Path, string(filepath.Separator), 2)
		if len(parts) < 2 {
			continue
		}
		slug := parts[0]
		proj, _, err := fs.ReadProject(ctx, slug)
		if err != nil {
			continue // skip unreadable projects
		}
		projects = append(projects, proj)
	}
	return projects, nil
}

// DeleteProject removes a project and all its features from storage.
func (fs *FeatureStorage) DeleteProject(ctx context.Context, projectSlug string) error {
	// First delete all features.
	features, err := fs.ListFeatures(ctx, projectSlug)
	if err != nil {
		return fmt.Errorf("list features for deletion: %w", err)
	}
	for _, f := range features {
		if delErr := fs.DeleteFeature(ctx, projectSlug, f.ID); delErr != nil {
			return fmt.Errorf("delete feature %s: %w", f.ID, delErr)
		}
	}
	// Then delete the project file.
	path := filepath.Join(projectSlug, helpers.ConfigFile)
	return fs.storageDelete(ctx, path)
}

// ---------- Feature operations ----------

// ReadFeature loads a feature by project slug and feature ID.
func (fs *FeatureStorage) ReadFeature(ctx context.Context, projectSlug, featureID string) (*types.FeatureData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.FeaturesDir, featureID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read feature %s/%s: %w", projectSlug, featureID, err)
	}

	feat, err := metadataToFeature(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse feature %s/%s: %w", projectSlug, featureID, err)
	}
	return feat, string(resp.Content), resp.Version, nil
}

// WriteFeature persists a feature to storage.
func (fs *FeatureStorage) WriteFeature(ctx context.Context, projectSlug, featureID string, data *types.FeatureData, body string, expectedVersion int64) (int64, error) {
	meta, err := featureToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode feature: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.FeaturesDir, featureID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListFeatures returns all features for a project.
func (fs *FeatureStorage) ListFeatures(ctx context.Context, projectSlug string) ([]*types.FeatureData, error) {
	prefix := filepath.Join(projectSlug, helpers.FeaturesDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}

	var features []*types.FeatureData
	for _, entry := range entries {
		// Extract feature ID from path like "my-project/features/FEAT-ABC.md"
		base := filepath.Base(entry.Path)
		featureID := strings.TrimSuffix(base, ".md")

		feat, _, _, err := fs.ReadFeature(ctx, projectSlug, featureID)
		if err != nil {
			continue // skip unreadable features
		}
		features = append(features, feat)
	}
	return features, nil
}

// DeleteFeature removes a feature from storage.
func (fs *FeatureStorage) DeleteFeature(ctx context.Context, projectSlug, featureID string) error {
	path := filepath.Join(projectSlug, helpers.FeaturesDir, featureID+".md")
	return fs.storageDelete(ctx, path)
}

// ---------- Low-level storage protocol ----------

func (fs *FeatureStorage) storageRead(ctx context.Context, path string) (*pluginv1.StorageReadResponse, error) {
	resp, err := fs.client.Send(ctx, &pluginv1.PluginRequest{
		RequestId: helpers.NewUUID(),
		Request: &pluginv1.PluginRequest_StorageRead{
			StorageRead: &pluginv1.StorageReadRequest{
				Path:        path,
				StorageType: "markdown",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	sr := resp.GetStorageRead()
	if sr == nil {
		return nil, fmt.Errorf("unexpected response type for storage read")
	}
	return sr, nil
}

func (fs *FeatureStorage) storageWrite(ctx context.Context, path string, metadata *structpb.Struct, content []byte, expectedVersion int64) (int64, error) {
	resp, err := fs.client.Send(ctx, &pluginv1.PluginRequest{
		RequestId: helpers.NewUUID(),
		Request: &pluginv1.PluginRequest_StorageWrite{
			StorageWrite: &pluginv1.StorageWriteRequest{
				Path:            path,
				Content:         content,
				Metadata:        metadata,
				ExpectedVersion: expectedVersion,
				StorageType:     "markdown",
			},
		},
	})
	if err != nil {
		return 0, err
	}
	sw := resp.GetStorageWrite()
	if sw == nil {
		return 0, fmt.Errorf("unexpected response type for storage write")
	}
	if !sw.Success {
		return 0, fmt.Errorf("storage write failed: %s", sw.Error)
	}
	return sw.NewVersion, nil
}

func (fs *FeatureStorage) storageDelete(ctx context.Context, path string) error {
	resp, err := fs.client.Send(ctx, &pluginv1.PluginRequest{
		RequestId: helpers.NewUUID(),
		Request: &pluginv1.PluginRequest_StorageDelete{
			StorageDelete: &pluginv1.StorageDeleteRequest{
				Path:        path,
				StorageType: "markdown",
			},
		},
	})
	if err != nil {
		return err
	}
	sd := resp.GetStorageDelete()
	if sd == nil {
		return fmt.Errorf("unexpected response type for storage delete")
	}
	if !sd.Success {
		return fmt.Errorf("storage delete failed")
	}
	return nil
}

func (fs *FeatureStorage) storageList(ctx context.Context, prefix, pattern string) ([]*pluginv1.StorageEntry, error) {
	resp, err := fs.client.Send(ctx, &pluginv1.PluginRequest{
		RequestId: helpers.NewUUID(),
		Request: &pluginv1.PluginRequest_StorageList{
			StorageList: &pluginv1.StorageListRequest{
				Prefix:      prefix,
				Pattern:     pattern,
				StorageType: "markdown",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	sl := resp.GetStorageList()
	if sl == nil {
		return nil, fmt.Errorf("unexpected response type for storage list")
	}
	return sl.Entries, nil
}

// ---------- WIP Config operations ----------

// ReadWIPConfig reads the WIP configuration version for a project.
func (fs *FeatureStorage) ReadWIPConfig(ctx context.Context, projectSlug string) (int64, error) {
	path := projectSlug + "/wip.json"
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return 0, err
	}
	return resp.Version, nil
}

// WriteWIPConfig writes the WIP configuration for a project.
func (fs *FeatureStorage) WriteWIPConfig(ctx context.Context, projectSlug string, meta *structpb.Struct, expectedVersion int64) error {
	path := projectSlug + "/wip.json"
	_, err := fs.storageWrite(ctx, path, meta, nil, expectedVersion)
	return err
}

// ReadWIPMetadata reads the WIP metadata struct for a project.
func (fs *FeatureStorage) ReadWIPMetadata(ctx context.Context, projectSlug string) (*structpb.Struct, error) {
	path := projectSlug + "/wip.json"
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, err
	}
	if resp.Metadata == nil {
		return nil, fmt.Errorf("no WIP metadata")
	}
	return resp.Metadata, nil
}

// ---------- Metadata conversion helpers ----------

func featureToMetadata(f *types.FeatureData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":          f.ID,
		"project_id":  f.ProjectID,
		"title":       f.Title,
		"description": f.Description,
		"status":      string(f.Status),
		"priority":    f.Priority,
		"version":     float64(f.Version),
		"created_at":  f.CreatedAt,
		"updated_at":  f.UpdatedAt,
	}
	if f.Assignee != "" {
		m["assignee"] = f.Assignee
	}
	if len(f.Labels) > 0 {
		labels := make([]any, len(f.Labels))
		for i, l := range f.Labels {
			labels[i] = l
		}
		m["labels"] = labels
	}
	if len(f.DependsOn) > 0 {
		deps := make([]any, len(f.DependsOn))
		for i, d := range f.DependsOn {
			deps[i] = d
		}
		m["depends_on"] = deps
	}
	if len(f.Blocks) > 0 {
		blocks := make([]any, len(f.Blocks))
		for i, b := range f.Blocks {
			blocks[i] = b
		}
		m["blocks"] = blocks
	}
	if f.Estimate != "" {
		m["estimate"] = f.Estimate
	}
	if f.Kind != "" {
		m["kind"] = string(f.Kind)
	}
	return structpb.NewStruct(m)
}

func metadataToFeature(meta *structpb.Struct) (*types.FeatureData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var f types.FeatureData
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func projectToMetadata(p *types.ProjectData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":          p.ID,
		"name":        p.Name,
		"slug":        p.Slug,
		"description": p.Description,
		"created_at":  p.CreatedAt,
		"updated_at":  p.UpdatedAt,
	}
	if p.Mode != "" {
		m["mode"] = string(p.Mode)
	}
	return structpb.NewStruct(m)
}

func metadataToProject(meta *structpb.Struct) (*types.ProjectData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var p types.ProjectData
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---------- Plan operations ----------

// ReadPlan loads a plan by project slug and plan ID.
func (fs *FeatureStorage) ReadPlan(ctx context.Context, projectSlug, planID string) (*types.PlanData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.PlansDir, planID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read plan %s/%s: %w", projectSlug, planID, err)
	}
	plan, err := metadataToPlan(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse plan %s/%s: %w", projectSlug, planID, err)
	}
	return plan, string(resp.Content), resp.Version, nil
}

// WritePlan persists a plan to storage.
func (fs *FeatureStorage) WritePlan(ctx context.Context, projectSlug, planID string, data *types.PlanData, body string, expectedVersion int64) (int64, error) {
	meta, err := planToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode plan: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.PlansDir, planID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListPlans returns all plans for a project.
func (fs *FeatureStorage) ListPlans(ctx context.Context, projectSlug string) ([]*types.PlanData, error) {
	prefix := filepath.Join(projectSlug, helpers.PlansDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	var plans []*types.PlanData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		planID := strings.TrimSuffix(base, ".md")
		plan, _, _, err := fs.ReadPlan(ctx, projectSlug, planID)
		if err != nil {
			continue
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// DeletePlan removes a plan from storage.
func (fs *FeatureStorage) DeletePlan(ctx context.Context, projectSlug, planID string) error {
	path := filepath.Join(projectSlug, helpers.PlansDir, planID+".md")
	return fs.storageDelete(ctx, path)
}

func planToMetadata(p *types.PlanData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":          p.ID,
		"project_id":  p.ProjectID,
		"title":       p.Title,
		"description": p.Description,
		"status":      string(p.Status),
		"version":     float64(p.Version),
		"created_at":  p.CreatedAt,
		"updated_at":  p.UpdatedAt,
	}
	if len(p.Features) > 0 {
		feats := make([]any, len(p.Features))
		for i, f := range p.Features {
			feats[i] = f
		}
		m["features"] = feats
	}
	return structpb.NewStruct(m)
}

func metadataToPlan(meta *structpb.Struct) (*types.PlanData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var p types.PlanData
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---------- Request operations ----------

// ReadRequest loads a request by project slug and request ID.
func (fs *FeatureStorage) ReadRequest(ctx context.Context, projectSlug, requestID string) (*types.RequestData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.RequestsDir, requestID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read request %s/%s: %w", projectSlug, requestID, err)
	}
	req, err := metadataToRequest(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse request %s/%s: %w", projectSlug, requestID, err)
	}
	return req, string(resp.Content), resp.Version, nil
}

// WriteRequest persists a request to storage.
func (fs *FeatureStorage) WriteRequest(ctx context.Context, projectSlug, requestID string, data *types.RequestData, body string, expectedVersion int64) (int64, error) {
	meta, err := requestToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode request: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.RequestsDir, requestID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListRequests returns all requests for a project.
func (fs *FeatureStorage) ListRequests(ctx context.Context, projectSlug string) ([]*types.RequestData, error) {
	prefix := filepath.Join(projectSlug, helpers.RequestsDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}
	var requests []*types.RequestData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		requestID := strings.TrimSuffix(base, ".md")
		r, _, _, err := fs.ReadRequest(ctx, projectSlug, requestID)
		if err != nil {
			continue
		}
		requests = append(requests, r)
	}
	return requests, nil
}

// DeleteRequest removes a request from storage.
func (fs *FeatureStorage) DeleteRequest(ctx context.Context, projectSlug, requestID string) error {
	path := filepath.Join(projectSlug, helpers.RequestsDir, requestID+".md")
	return fs.storageDelete(ctx, path)
}

// ---------- Person operations ----------

// ReadPerson loads a person by project slug and person ID.
func (fs *FeatureStorage) ReadPerson(ctx context.Context, projectSlug, personID string) (*types.PersonData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.PersonsDir, personID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read person %s/%s: %w", projectSlug, personID, err)
	}
	person, err := metadataToPerson(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse person %s/%s: %w", projectSlug, personID, err)
	}
	return person, string(resp.Content), resp.Version, nil
}

// WritePerson persists a person to storage.
func (fs *FeatureStorage) WritePerson(ctx context.Context, projectSlug, personID string, data *types.PersonData, body string, expectedVersion int64) (int64, error) {
	meta, err := personToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode person: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.PersonsDir, personID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListPersons returns all persons for a project.
func (fs *FeatureStorage) ListPersons(ctx context.Context, projectSlug string) ([]*types.PersonData, error) {
	prefix := filepath.Join(projectSlug, helpers.PersonsDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list persons: %w", err)
	}
	var persons []*types.PersonData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		personID := strings.TrimSuffix(base, ".md")
		person, _, _, err := fs.ReadPerson(ctx, projectSlug, personID)
		if err != nil {
			continue
		}
		persons = append(persons, person)
	}
	return persons, nil
}

// DeletePerson removes a person from storage.
func (fs *FeatureStorage) DeletePerson(ctx context.Context, projectSlug, personID string) error {
	path := filepath.Join(projectSlug, helpers.PersonsDir, personID+".md")
	return fs.storageDelete(ctx, path)
}

// ---------- Assignment Rule operations ----------

// ReadAssignmentRule loads an assignment rule by project slug and rule ID.
func (fs *FeatureStorage) ReadAssignmentRule(ctx context.Context, projectSlug, ruleID string) (*types.AssignmentRuleData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.AssignmentRulesDir, ruleID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read assignment rule %s/%s: %w", projectSlug, ruleID, err)
	}
	rule, err := metadataToAssignmentRule(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse assignment rule %s/%s: %w", projectSlug, ruleID, err)
	}
	return rule, string(resp.Content), resp.Version, nil
}

// WriteAssignmentRule persists an assignment rule to storage.
func (fs *FeatureStorage) WriteAssignmentRule(ctx context.Context, projectSlug, ruleID string, data *types.AssignmentRuleData, body string, expectedVersion int64) (int64, error) {
	meta, err := assignmentRuleToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode assignment rule: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.AssignmentRulesDir, ruleID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListAssignmentRules returns all assignment rules for a project.
func (fs *FeatureStorage) ListAssignmentRules(ctx context.Context, projectSlug string) ([]*types.AssignmentRuleData, error) {
	prefix := filepath.Join(projectSlug, helpers.AssignmentRulesDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list assignment rules: %w", err)
	}
	var rules []*types.AssignmentRuleData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		ruleID := strings.TrimSuffix(base, ".md")
		rule, _, _, err := fs.ReadAssignmentRule(ctx, projectSlug, ruleID)
		if err != nil {
			continue
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// DeleteAssignmentRule removes an assignment rule from storage.
func (fs *FeatureStorage) DeleteAssignmentRule(ctx context.Context, projectSlug, ruleID string) error {
	path := filepath.Join(projectSlug, helpers.AssignmentRulesDir, ruleID+".md")
	return fs.storageDelete(ctx, path)
}

func requestToMetadata(r *types.RequestData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":          r.ID,
		"project_id":  r.ProjectID,
		"title":       r.Title,
		"description": r.Description,
		"kind":        r.Kind,
		"status":      string(r.Status),
		"priority":    r.Priority,
		"version":     float64(r.Version),
		"created_at":  r.CreatedAt,
		"updated_at":  r.UpdatedAt,
	}
	if r.ConvertedTo != "" {
		m["converted_to"] = r.ConvertedTo
	}
	return structpb.NewStruct(m)
}

func metadataToRequest(meta *structpb.Struct) (*types.RequestData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var r types.RequestData
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ---------- Person metadata conversion ----------

func personToMetadata(p *types.PersonData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":         p.ID,
		"project_id": p.ProjectID,
		"name":       p.Name,
		"role":       string(p.Role),
		"status":     string(p.Status),
		"version":    float64(p.Version),
		"created_at": p.CreatedAt,
		"updated_at": p.UpdatedAt,
	}
	if p.Email != "" {
		m["email"] = p.Email
	}
	if p.Bio != "" {
		m["bio"] = p.Bio
	}
	if p.GithubEmail != "" {
		m["github_email"] = p.GithubEmail
	}
	if len(p.Integrations) > 0 {
		integ := make(map[string]any, len(p.Integrations))
		for k, v := range p.Integrations {
			integ[k] = v
		}
		m["integrations"] = integ
	}
	if len(p.Labels) > 0 {
		labels := make([]any, len(p.Labels))
		for i, l := range p.Labels {
			labels[i] = l
		}
		m["labels"] = labels
	}
	return structpb.NewStruct(m)
}

func metadataToPerson(meta *structpb.Struct) (*types.PersonData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var p types.PersonData
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ---------- Assignment Rule metadata conversion ----------

func assignmentRuleToMetadata(r *types.AssignmentRuleData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":         r.ID,
		"project_id": r.ProjectID,
		"kind":       r.Kind,
		"person_id":  r.PersonID,
		"version":    float64(r.Version),
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	}
	return structpb.NewStruct(m)
}

func metadataToAssignmentRule(meta *structpb.Struct) (*types.AssignmentRuleData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var r types.AssignmentRuleData
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ---------- Hypothesis operations ----------

// ReadHypothesis loads a hypothesis by project slug and hypothesis ID.
func (fs *FeatureStorage) ReadHypothesis(ctx context.Context, projectSlug, hypoID string) (*types.HypothesisData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.HypothesesDir, hypoID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read hypothesis %s/%s: %w", projectSlug, hypoID, err)
	}
	hypo, err := metadataToHypothesis(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse hypothesis %s/%s: %w", projectSlug, hypoID, err)
	}
	return hypo, string(resp.Content), resp.Version, nil
}

// WriteHypothesis persists a hypothesis to storage.
func (fs *FeatureStorage) WriteHypothesis(ctx context.Context, projectSlug, hypoID string, data *types.HypothesisData, body string, expectedVersion int64) (int64, error) {
	meta, err := hypothesisToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode hypothesis: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.HypothesesDir, hypoID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListHypotheses returns all hypotheses for a project.
func (fs *FeatureStorage) ListHypotheses(ctx context.Context, projectSlug string) ([]*types.HypothesisData, error) {
	prefix := filepath.Join(projectSlug, helpers.HypothesesDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list hypotheses: %w", err)
	}
	var hypotheses []*types.HypothesisData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		hypoID := strings.TrimSuffix(base, ".md")
		hypo, _, _, err := fs.ReadHypothesis(ctx, projectSlug, hypoID)
		if err != nil {
			continue
		}
		hypotheses = append(hypotheses, hypo)
	}
	return hypotheses, nil
}

// DeleteHypothesis removes a hypothesis from storage.
func (fs *FeatureStorage) DeleteHypothesis(ctx context.Context, projectSlug, hypoID string) error {
	path := filepath.Join(projectSlug, helpers.HypothesesDir, hypoID+".md")
	return fs.storageDelete(ctx, path)
}

func hypothesisToMetadata(h *types.HypothesisData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":          h.ID,
		"project_id":  h.ProjectID,
		"title":       h.Title,
		"problem":     h.Problem,
		"target_user": h.TargetUser,
		"assumption":  h.Assumption,
		"status":      string(h.Status),
		"version":     float64(h.Version),
		"created_at":  h.CreatedAt,
		"updated_at":  h.UpdatedAt,
	}
	if h.CycleID != "" {
		m["cycle_id"] = h.CycleID
	}
	if h.RefinedFrom != "" {
		m["refined_from"] = h.RefinedFrom
	}
	if len(h.Experiments) > 0 {
		exps := make([]any, len(h.Experiments))
		for i, e := range h.Experiments {
			exps[i] = e
		}
		m["experiments"] = exps
	}
	if len(h.Labels) > 0 {
		labels := make([]any, len(h.Labels))
		for i, l := range h.Labels {
			labels[i] = l
		}
		m["labels"] = labels
	}
	return structpb.NewStruct(m)
}

func metadataToHypothesis(meta *structpb.Struct) (*types.HypothesisData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var h types.HypothesisData
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

// ---------- Experiment operations ----------

// ReadExperiment loads an experiment by project slug and experiment ID.
func (fs *FeatureStorage) ReadExperiment(ctx context.Context, projectSlug, exprID string) (*types.ExperimentData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.ExperimentsDir, exprID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read experiment %s/%s: %w", projectSlug, exprID, err)
	}
	expr, err := metadataToExperiment(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse experiment %s/%s: %w", projectSlug, exprID, err)
	}
	return expr, string(resp.Content), resp.Version, nil
}

// WriteExperiment persists an experiment to storage.
func (fs *FeatureStorage) WriteExperiment(ctx context.Context, projectSlug, exprID string, data *types.ExperimentData, body string, expectedVersion int64) (int64, error) {
	meta, err := experimentToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode experiment: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.ExperimentsDir, exprID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListExperiments returns all experiments for a project.
func (fs *FeatureStorage) ListExperiments(ctx context.Context, projectSlug string) ([]*types.ExperimentData, error) {
	prefix := filepath.Join(projectSlug, helpers.ExperimentsDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list experiments: %w", err)
	}
	var experiments []*types.ExperimentData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		exprID := strings.TrimSuffix(base, ".md")
		expr, _, _, err := fs.ReadExperiment(ctx, projectSlug, exprID)
		if err != nil {
			continue
		}
		experiments = append(experiments, expr)
	}
	return experiments, nil
}

// DeleteExperiment removes an experiment from storage.
func (fs *FeatureStorage) DeleteExperiment(ctx context.Context, projectSlug, exprID string) error {
	path := filepath.Join(projectSlug, helpers.ExperimentsDir, exprID+".md")
	return fs.storageDelete(ctx, path)
}

func experimentToMetadata(e *types.ExperimentData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":             e.ID,
		"project_id":     e.ProjectID,
		"hypothesis_id":  e.HypothesisID,
		"title":          e.Title,
		"kind":           string(e.Kind),
		"question":       e.Question,
		"method":         e.Method,
		"success_signal": e.SuccessSignal,
		"kill_condition": e.KillCondition,
		"status":         string(e.Status),
		"version":        float64(e.Version),
		"created_at":     e.CreatedAt,
		"updated_at":     e.UpdatedAt,
	}
	if e.CycleID != "" {
		m["cycle_id"] = e.CycleID
	}
	if e.Outcome != "" {
		m["outcome"] = e.Outcome
	}
	if e.KillTriggered {
		m["kill_triggered"] = true
	}
	if len(e.Signals) > 0 {
		sigs := make([]any, len(e.Signals))
		for i, s := range e.Signals {
			sigs[i] = map[string]any{
				"type":        string(s.Type),
				"metric":      s.Metric,
				"expected":    s.Expected,
				"actual":      s.Actual,
				"confidence":  s.Confidence,
				"recorded_at": s.RecordedAt,
			}
		}
		m["signals"] = sigs
	}
	if len(e.SpawnedFeatures) > 0 {
		feats := make([]any, len(e.SpawnedFeatures))
		for i, f := range e.SpawnedFeatures {
			feats[i] = f
		}
		m["spawned_features"] = feats
	}
	if len(e.Labels) > 0 {
		labels := make([]any, len(e.Labels))
		for i, l := range e.Labels {
			labels[i] = l
		}
		m["labels"] = labels
	}
	return structpb.NewStruct(m)
}

func metadataToExperiment(meta *structpb.Struct) (*types.ExperimentData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var e types.ExperimentData
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// ---------- Discovery Cycle operations ----------

// ReadDiscoveryCycle loads a discovery cycle by project slug and cycle ID.
func (fs *FeatureStorage) ReadDiscoveryCycle(ctx context.Context, projectSlug, cycleID string) (*types.DiscoveryCycleData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.DiscoveryCyclesDir, cycleID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read discovery cycle %s/%s: %w", projectSlug, cycleID, err)
	}
	cycle, err := metadataToDiscoveryCycle(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse discovery cycle %s/%s: %w", projectSlug, cycleID, err)
	}
	return cycle, string(resp.Content), resp.Version, nil
}

// WriteDiscoveryCycle persists a discovery cycle to storage.
func (fs *FeatureStorage) WriteDiscoveryCycle(ctx context.Context, projectSlug, cycleID string, data *types.DiscoveryCycleData, body string, expectedVersion int64) (int64, error) {
	meta, err := discoveryCycleToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode discovery cycle: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.DiscoveryCyclesDir, cycleID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListDiscoveryCycles returns all discovery cycles for a project.
func (fs *FeatureStorage) ListDiscoveryCycles(ctx context.Context, projectSlug string) ([]*types.DiscoveryCycleData, error) {
	prefix := filepath.Join(projectSlug, helpers.DiscoveryCyclesDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list discovery cycles: %w", err)
	}
	var cycles []*types.DiscoveryCycleData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		cycleID := strings.TrimSuffix(base, ".md")
		cycle, _, _, err := fs.ReadDiscoveryCycle(ctx, projectSlug, cycleID)
		if err != nil {
			continue
		}
		cycles = append(cycles, cycle)
	}
	return cycles, nil
}

// DeleteDiscoveryCycle removes a discovery cycle from storage.
func (fs *FeatureStorage) DeleteDiscoveryCycle(ctx context.Context, projectSlug, cycleID string) error {
	path := filepath.Join(projectSlug, helpers.DiscoveryCyclesDir, cycleID+".md")
	return fs.storageDelete(ctx, path)
}

func discoveryCycleToMetadata(c *types.DiscoveryCycleData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":         c.ID,
		"project_id": c.ProjectID,
		"title":      c.Title,
		"goal":       c.Goal,
		"start_date": c.StartDate,
		"end_date":   c.EndDate,
		"status":     string(c.Status),
		"version":    float64(c.Version),
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
	if c.Learnings != "" {
		m["learnings"] = c.Learnings
	}
	if c.Decision != "" {
		m["decision"] = c.Decision
	}
	if len(c.Hypotheses) > 0 {
		hypos := make([]any, len(c.Hypotheses))
		for i, h := range c.Hypotheses {
			hypos[i] = h
		}
		m["hypotheses"] = hypos
	}
	if len(c.Experiments) > 0 {
		exps := make([]any, len(c.Experiments))
		for i, e := range c.Experiments {
			exps[i] = e
		}
		m["experiments"] = exps
	}
	return structpb.NewStruct(m)
}

func metadataToDiscoveryCycle(meta *structpb.Struct) (*types.DiscoveryCycleData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var c types.DiscoveryCycleData
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ---------- Discovery Review operations ----------

// ReadDiscoveryReview loads a discovery review by project slug and review ID.
func (fs *FeatureStorage) ReadDiscoveryReview(ctx context.Context, projectSlug, reviewID string) (*types.DiscoveryReviewData, string, int64, error) {
	path := filepath.Join(projectSlug, helpers.DiscoveryReviewsDir, reviewID+".md")
	resp, err := fs.storageRead(ctx, path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("read discovery review %s/%s: %w", projectSlug, reviewID, err)
	}
	review, err := metadataToDiscoveryReview(resp.Metadata)
	if err != nil {
		return nil, "", 0, fmt.Errorf("parse discovery review %s/%s: %w", projectSlug, reviewID, err)
	}
	return review, string(resp.Content), resp.Version, nil
}

// WriteDiscoveryReview persists a discovery review to storage.
func (fs *FeatureStorage) WriteDiscoveryReview(ctx context.Context, projectSlug, reviewID string, data *types.DiscoveryReviewData, body string, expectedVersion int64) (int64, error) {
	meta, err := discoveryReviewToMetadata(data)
	if err != nil {
		return 0, fmt.Errorf("encode discovery review: %w", err)
	}
	path := filepath.Join(projectSlug, helpers.DiscoveryReviewsDir, reviewID+".md")
	return fs.storageWrite(ctx, path, meta, []byte(body), expectedVersion)
}

// ListDiscoveryReviews returns all discovery reviews for a project.
func (fs *FeatureStorage) ListDiscoveryReviews(ctx context.Context, projectSlug string) ([]*types.DiscoveryReviewData, error) {
	prefix := filepath.Join(projectSlug, helpers.DiscoveryReviewsDir) + string(filepath.Separator)
	entries, err := fs.storageList(ctx, prefix, "*.md")
	if err != nil {
		return nil, fmt.Errorf("list discovery reviews: %w", err)
	}
	var reviews []*types.DiscoveryReviewData
	for _, entry := range entries {
		base := filepath.Base(entry.Path)
		reviewID := strings.TrimSuffix(base, ".md")
		review, _, _, err := fs.ReadDiscoveryReview(ctx, projectSlug, reviewID)
		if err != nil {
			continue
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

// DeleteDiscoveryReview removes a discovery review from storage.
func (fs *FeatureStorage) DeleteDiscoveryReview(ctx context.Context, projectSlug, reviewID string) error {
	path := filepath.Join(projectSlug, helpers.DiscoveryReviewsDir, reviewID+".md")
	return fs.storageDelete(ctx, path)
}

func discoveryReviewToMetadata(r *types.DiscoveryReviewData) (*structpb.Struct, error) {
	m := map[string]any{
		"id":         r.ID,
		"project_id": r.ProjectID,
		"cycle_id":   r.CycleID,
		"title":      r.Title,
		"version":    float64(r.Version),
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	}
	if r.Surprises != "" {
		m["surprises"] = r.Surprises
	}
	if r.WrongAbout != "" {
		m["wrong_about"] = r.WrongAbout
	}
	if r.TransitionReady {
		m["transition_ready"] = true
	}
	if len(r.Items) > 0 {
		items := make([]any, len(r.Items))
		for i, item := range r.Items {
			items[i] = map[string]any{
				"item_id":   item.ItemID,
				"item_type": item.ItemType,
				"decision":  string(item.Decision),
				"rationale": item.Rationale,
			}
		}
		m["items"] = items
	}
	return structpb.NewStruct(m)
}

func metadataToDiscoveryReview(meta *structpb.Struct) (*types.DiscoveryReviewData, error) {
	if meta == nil {
		return nil, fmt.Errorf("no metadata")
	}
	raw, err := json.Marshal(meta.AsMap())
	if err != nil {
		return nil, err
	}
	var r types.DiscoveryReviewData
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
