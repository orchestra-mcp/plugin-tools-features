package toolsfeatures

import (
	"context"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/plugin-tools-features/internal"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
	"github.com/orchestra-mcp/sdk-go/plugin"
)

// Sender is the interface that the in-process router satisfies.
type Sender interface {
	Send(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error)
}

// Register adds all 35 feature workflow tools to the builder.
func Register(builder *plugin.PluginBuilder, sender Sender) {
	store := storage.NewFeatureStorage(sender)
	fp := &internal.FeaturesPlugin{Storage: store}
	fp.RegisterTools(builder)
}
