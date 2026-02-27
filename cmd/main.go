// Command tools-features is the entry point for the tools.features plugin
// binary. It creates the plugin with all 34 feature-driven workflow tools and
// connects to the orchestrator for storage access.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	pluginv1 "github.com/orchestra-mcp/gen-go/orchestra/plugin/v1"
	"github.com/orchestra-mcp/sdk-go/plugin"
	"github.com/orchestra-mcp/plugin-tools-features/internal"
	"github.com/orchestra-mcp/plugin-tools-features/internal/storage"
)

func main() {
	// The builder is constructed first so we can register tools before building.
	builder := plugin.New("tools.features").
		Version("0.1.0").
		Description("Feature-driven workflow engine with 34 core tools").
		Author("Orchestra").
		Binary("tools-features").
		NeedsStorage("markdown")

	// Create a placeholder storage that will be replaced once the orchestrator
	// client connects. For tool registration we need the storage reference up
	// front; the underlying client is set lazily when Run dials the orchestrator.
	//
	// In production, the tools access storage through the orchestrator QUIC
	// client. The Plugin.OrchestratorClient() method returns the client after
	// Run has connected.
	adapter := &clientAdapter{}
	store := storage.NewFeatureStorage(adapter)

	fp := &internal.FeaturesPlugin{
		Storage: store,
	}
	fp.RegisterTools(builder)

	p := builder.BuildWithTools()
	p.ParseFlags()

	// Wire the adapter to the plugin so it can retrieve the orchestrator
	// client after Run connects.
	adapter.plugin = p

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := p.Run(ctx); err != nil {
		log.Fatalf("tools.features: %v", err)
	}
}

// clientAdapter implements storage.StorageClient by forwarding to the plugin's
// orchestrator client. This allows tool handlers to use storage operations
// through the QUIC connection that is established during Run.
type clientAdapter struct {
	plugin *plugin.Plugin
}

func (a *clientAdapter) Send(ctx context.Context, req *pluginv1.PluginRequest) (*pluginv1.PluginResponse, error) {
	client := a.plugin.OrchestratorClient()
	if client == nil {
		return nil, fmt.Errorf("orchestrator client not connected")
	}
	return client.Send(ctx, req)
}
