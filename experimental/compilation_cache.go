package experimental

import (
	"context"
	"github.com/tetratelabs/wazero/internal/compilationcache"
)

// WithCompilationCacheDirName TODO
func WithCompilationCacheDirName(ctx context.Context, dirname string) context.Context {
	if len(dirname) != 0 {
		ctx = context.WithValue(ctx, compilationcache.FileCachePathKey{}, dirname)
	}
	return ctx
}
