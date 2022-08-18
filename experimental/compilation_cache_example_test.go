package experimental_test

import (
	"context"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
)

// This is a basic example of using the file system compilation cache via WithCompilationCacheDirName.
// The main goal is to show how it is configured.
func Example_withCompilationCacheDirName() {
	// Prepare a cache directory.
	cacheDir, err := os.MkdirTemp("", "example")
	if err != nil {
		log.Panicln(err)
	}
	defer os.RemoveAll(cacheDir)

	// Append the directory into the context for configuration.
	ctx := experimental.WithCompilationCacheDirName(context.Background(), cacheDir)

	// Repeat newRuntimeCompileDestroy with the same cache directory.
	newRuntimeCompileDestroy(ctx)
	// After the second invocation, the actual compilation doesn't happen,
	// and instead, the persisted file cache is re-used.
	newRuntimeCompileDestroy(ctx)
	newRuntimeCompileDestroy(ctx)

	// Output:
	//
}

// newRuntimeCompileDestroy creates a new wazero.Runtime, compile a binary, and then delete the runtime.
func newRuntimeCompileDestroy(ctx context.Context) {
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created except the file system cache.

	_, err := r.CompileModule(ctx, fsWasm, wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}
}
