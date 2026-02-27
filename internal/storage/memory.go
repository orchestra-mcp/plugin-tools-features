package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fileEntry represents a stored file in memory.
type fileEntry struct {
	content  []byte
	metadata *structpb.Struct
	version  int64
}

// InMemoryStorage implements StorageClient using an in-memory map. This is used
// for testing so that tool handlers can be exercised without a running QUIC
// orchestrator or filesystem.
type InMemoryStorage struct {
	mu    sync.Mutex
	files map[string]*fileEntry
}

// NewInMemoryStorage creates a new in-memory storage backend.
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		files: make(map[string]*fileEntry),
	}
}

// Send dispatches a PluginRequest to the appropriate in-memory handler based
// on the request type.
func (m *InMemoryStorage) Send(_ context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
	resp := &pluginv1.PluginResponse{
		RequestId: req.RequestId,
	}

	switch r := req.Request.(type) {
	case *pluginv1.PluginRequest_StorageRead:
		result, err := m.handleRead(r.StorageRead)
		if err != nil {
			return nil, err
		}
		resp.Response = &pluginv1.PluginResponse_StorageRead{StorageRead: result}

	case *pluginv1.PluginRequest_StorageWrite:
		result, err := m.handleWrite(r.StorageWrite)
		if err != nil {
			return nil, err
		}
		resp.Response = &pluginv1.PluginResponse_StorageWrite{StorageWrite: result}

	case *pluginv1.PluginRequest_StorageDelete:
		result := m.handleDelete(r.StorageDelete)
		resp.Response = &pluginv1.PluginResponse_StorageDelete{StorageDelete: result}

	case *pluginv1.PluginRequest_StorageList:
		result := m.handleList(r.StorageList)
		resp.Response = &pluginv1.PluginResponse_StorageList{StorageList: result}

	default:
		return nil, fmt.Errorf("unsupported request type in InMemoryStorage")
	}

	return resp, nil
}

func (m *InMemoryStorage) handleRead(req *pluginv1.StorageReadRequest) (*pluginv1.StorageReadResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.files[req.Path]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", req.Path)
	}

	// Clone metadata to avoid mutation.
	var meta *structpb.Struct
	if entry.metadata != nil {
		raw, _ := json.Marshal(entry.metadata.AsMap())
		var metaMap map[string]any
		_ = json.Unmarshal(raw, &metaMap)
		meta, _ = structpb.NewStruct(metaMap)
	}

	contentCopy := make([]byte, len(entry.content))
	copy(contentCopy, entry.content)

	return &pluginv1.StorageReadResponse{
		Content:  contentCopy,
		Metadata: meta,
		Version:  entry.version,
	}, nil
}

func (m *InMemoryStorage) handleWrite(req *pluginv1.StorageWriteRequest) (*pluginv1.StorageWriteResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.files[req.Path]

	if req.ExpectedVersion == 0 {
		// Create new: fail if the file already exists.
		if exists {
			return &pluginv1.StorageWriteResponse{
				Success: false,
				Error:   "file already exists (expected_version=0 means create new)",
			}, nil
		}
	} else {
		// Update: expected version must match current version.
		if !exists {
			return &pluginv1.StorageWriteResponse{
				Success: false,
				Error:   fmt.Sprintf("file not found for update: %s", req.Path),
			}, nil
		}
		if existing.version != req.ExpectedVersion {
			return &pluginv1.StorageWriteResponse{
				Success: false,
				Error:   fmt.Sprintf("version conflict: expected %d, current %d", req.ExpectedVersion, existing.version),
			}, nil
		}
	}

	var newVersion int64
	if exists {
		newVersion = existing.version + 1
	} else {
		newVersion = 1
	}

	// Clone metadata.
	var meta *structpb.Struct
	if req.Metadata != nil {
		raw, _ := json.Marshal(req.Metadata.AsMap())
		var metaMap map[string]any
		_ = json.Unmarshal(raw, &metaMap)
		meta, _ = structpb.NewStruct(metaMap)
	}

	contentCopy := make([]byte, len(req.Content))
	copy(contentCopy, req.Content)

	m.files[req.Path] = &fileEntry{
		content:  contentCopy,
		metadata: meta,
		version:  newVersion,
	}

	return &pluginv1.StorageWriteResponse{
		Success:    true,
		NewVersion: newVersion,
	}, nil
}

func (m *InMemoryStorage) handleDelete(req *pluginv1.StorageDeleteRequest) *pluginv1.StorageDeleteResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.files[req.Path]; !ok {
		return &pluginv1.StorageDeleteResponse{Success: false}
	}

	delete(m.files, req.Path)
	return &pluginv1.StorageDeleteResponse{Success: true}
}

func (m *InMemoryStorage) handleList(req *pluginv1.StorageListRequest) *pluginv1.StorageListResponse {
	m.mu.Lock()
	defer m.mu.Unlock()

	pattern := req.Pattern
	if pattern == "" {
		pattern = "*.md"
	}

	var entries []*pluginv1.StorageEntry
	for path, entry := range m.files {
		// Check prefix match.
		if req.Prefix != "" && !strings.HasPrefix(path, req.Prefix) {
			continue
		}

		// Check pattern match on filename.
		base := filepath.Base(path)
		matched, err := filepath.Match(pattern, base)
		if err != nil || !matched {
			continue
		}

		entries = append(entries, &pluginv1.StorageEntry{
			Path:       path,
			Size:       int64(len(entry.content)),
			Version:    entry.version,
			ModifiedAt: timestamppb.Now(),
		})
	}

	return &pluginv1.StorageListResponse{Entries: entries}
}
