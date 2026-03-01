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
