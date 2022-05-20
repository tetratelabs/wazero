package experimental

import (
	"context"
	"io/fs"

	"github.com/tetratelabs/wazero/api"
	internalfs "github.com/tetratelabs/wazero/internal/fs"
)

// WithFS overrides fs.FS in the context-based manner. Caller needs to take responsibility for closing the filesystem.
func WithFS(ctx context.Context, fs fs.FS) (context.Context, api.Closer, error) {
	fsConfig := internalfs.NewFSConfig().WithFS(fs)
	preopens, err := fsConfig.Preopens()
	if err != nil {
		return nil, nil, err
	}

	fsCtx := internalfs.NewContext(preopens)
	return context.WithValue(ctx, internalfs.Key{}, fsCtx), fsCtx, nil
}
