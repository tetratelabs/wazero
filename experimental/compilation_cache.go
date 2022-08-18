package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/compilationcache"
)

// WithCompilationCacheDirName configures the destination directory of the compilation cache.
// Regardless of the usage of this, the compiled functions are cached in memory, but its lifetime is
// bound to the lifetime of wazero.Runtime or wazero.CompiledModule.
//
// With the given non-empty directory, wazero persists the cache into the directory and that cache
// will be used as long as the running wazero version match the version of compilation wazero.
//
// A cache is only valid for use in one wazero.Runtime at a time. Concurrent use
// of a wazero.Runtime is supported, but multiple runtimes must not share the
// same directory.
//
// Note: The embedder must safeguard this directory from external changes.
//
// Usage:
//
//	ctx := experimental.WithCompilationCacheDirName(context.Background(), "/home/me/.cache/wazero")
//	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
func WithCompilationCacheDirName(ctx context.Context, dirname string) context.Context {
	if len(dirname) != 0 {
		ctx = context.WithValue(ctx, compilationcache.FileCachePathKey{}, dirname)
	}
	return ctx
}
