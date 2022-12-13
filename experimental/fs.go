package experimental

import (
	"context"
	"io/fs"

	"github.com/tetratelabs/wazero/api"
	internalfs "github.com/tetratelabs/wazero/internal/sys"
)

// WithFS overrides fs.FS in the context-based manner. Caller needs to take
// responsibility for closing the returned api.Closer.
//
// Note: This has the same effect as the same function on wazero.ModuleConfig.
func WithFS(ctx context.Context, fs fs.FS) (context.Context, api.Closer, error) {
	if fs == nil {
		fs = internalfs.EmptyFS
	}
	if fsCtx, err := internalfs.NewFSContext(fs); err != nil {
		return nil, nil, err
	} else {
		return context.WithValue(ctx, internalfs.FSKey{}, fsCtx), fsCtx, err
	}
}
