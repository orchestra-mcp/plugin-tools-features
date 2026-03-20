package toolsfeatures

import (
	"context"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/plugin"
	"github.com/orchestra-mcp/sdk-go/workflow"
)

// Sender is the interface that the in-process router satisfies.
type Sender interface {
	Send(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
}

// RegisterOption configures optional dependencies for the tools.features plugin.
type RegisterOption func(*registerConfig)

type registerConfig struct {
	engine  *workflow.Engine
	catalog plugin.ToolCatalog
}

// WithEngine sets a custom workflow engine (default: DefaultEngine).
func WithEngine(eng *workflow.Engine) RegisterOption {
	return func(c *registerConfig) {
		c.engine = eng
	}
}

// WithCatalog sets the tool catalog for MCP registry tools.
func WithCatalog(cat plugin.ToolCatalog) RegisterOption {
	return func(c *registerConfig) {
		c.catalog = cat
	}
}

// Register adds all feature workflow tools to the builder.
// An optional *workflow.Engine may be passed as the third argument; if absent
// or nil, the built-in DefaultEngine() is used so existing behaviour is preserved.
//
// Additional options can be passed via RegisterOption funcs after the engine
// (backwards-compatible with existing callers that pass *workflow.Engine).
func Register(builder *plugin.PluginBuilder, sender Sender, args ...any) {
	cfg := &registerConfig{}

	// Parse variadic args: supports legacy (*workflow.Engine) and new (RegisterOption).
	for _, arg := range args {
		switch v := arg.(type) {
		case *workflow.Engine:
			if v != nil {
				cfg.engine = v
			}
		case RegisterOption:
			v(cfg)
		}
	}

	if cfg.engine == nil {
		cfg.engine = workflow.DefaultEngine()
	}

	resolver := workflow.NewResolver(cfg.engine)

	store := storage.NewFeatureStorage(sender)
	fp := &internal.FeaturesPlugin{
		Storage:  store,
		Engine:   cfg.engine,
		Resolver: resolver,
		Catalog:  cfg.catalog,
	}
	fp.RegisterTools(builder)
}
