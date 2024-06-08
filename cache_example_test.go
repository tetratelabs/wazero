package wazero_test

import (
	"context"
	_ "embed"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
)

// This is a basic example of using the file system compilation cache via wazero.NewCompilationCacheWithDir.
// The main goal is to show how it is configured.
func Example_compileCache() {
	// Prepare a cache directory.
	cacheDir, err := os.MkdirTemp("", "example")
	if err != nil {
		log.Panicln(err)
	}
	defer os.RemoveAll(cacheDir)

	ctx := context.Background()

	// Create a runtime config which shares a compilation cache directory.
	cache := newCompilationCacheWithDir(cacheDir)
	defer cache.Close(ctx)
	config := wazero.NewRuntimeConfig().WithCompilationCache(cache)

	// Using the same wazero.CompilationCache instance allows the in-memory cache sharing.
	newRuntimeCompileClose(ctx, config)
	newRuntimeCompileClose(ctx, config)

	// Since the above stored compiled functions to disk as well, below won't compile from scratch.
	// Instead, compilation result stored in the directory is re-used.
	newRuntimeCompileClose(ctx, config.WithCompilationCache(newCompilationCacheWithDir(cacheDir)))
	newRuntimeCompileClose(ctx, config.WithCompilationCache(newCompilationCacheWithDir(cacheDir)))

	// Output:
	//
}

func newCompilationCacheWithDir(cacheDir string) wazero.CompilationCache {
	cache, err := wazero.NewCompilationCacheWithDir(cacheDir)
	if err != nil {
		log.Panicln(err)
	}
	return cache
}

// newRuntimeCompileClose creates a new wazero.Runtime, compile a binary, and then delete the runtime.
func newRuntimeCompileClose(ctx context.Context, config wazero.RuntimeConfig) {
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer r.Close(ctx) // This closes everything this Runtime created except the file system cache.

	_, err := r.CompileModule(ctx, addWasm)
	if err != nil {
		log.Panicln(err)
	}
}
