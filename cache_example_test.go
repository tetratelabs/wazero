package wazero_test

import (
	"context"
	_ "embed"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
)

// This is a basic example of using the file system compilation cache via WithCompilationCacheDirName.
// The main goal is to show how it is configured.
func Example_withCache() {
	// Prepare a cache directory.
	cacheDir, err := os.MkdirTemp("", "example")
	if err != nil {
		log.Panicln(err)
	}
	defer os.RemoveAll(cacheDir)

	ctx := context.Background()

	// Creates a new cache context with the file cache directory configured.
	cache := wazero.NewCache()
	defer cache.Close(ctx)
	if err := cache.WithCompilationCacheDirName(cacheDir); err != nil {
		log.Fatal(err)
	}

	// Creates a runtime configuration to create multiple runtimes.
	config := wazero.NewRuntimeConfig().WithCompileCache(cache)

	// Repeat newRuntimeCompileClose with the same cache directory.
	newRuntimeCompileClose(ctx, config)
	// Since the above stored compiled functions to dist, below won't compile.
	// Instead, code stored in the file cache is re-used.
	newRuntimeCompileClose(ctx, config)
	newRuntimeCompileClose(ctx, config)

	// Output:
	//
}

// newRuntimeCompileDestroy creates a new wazero.Runtime, compile a binary, and then delete the runtime.
func newRuntimeCompileClose(ctx context.Context, config wazero.RuntimeConfig) {
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer r.Close(ctx) // This closes everything this Runtime created except the file system cache.

	_, err := r.CompileModule(ctx, addWasm)
	if err != nil {
		log.Panicln(err)
	}
}
