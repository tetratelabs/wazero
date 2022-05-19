package fs

import (
	"context"
	"io/fs"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// FSKey is a context.Context Value key. It allows overriding fs.FS for WASI.
//
// See https://github.com/tetratelabs/wazero/issues/491
type FSKey struct{}

// WithFS overrides fs.FS in the context-based manner. Caller needs to take responsibility for closing the filesystem.
func WithFS(ctx context.Context, fs fs.FS) (context.Context, api.Closer, error) {
	fsConfig := wasm.NewFSConfig().WithFS(fs)
	preopens, err := fsConfig.Preopens()
	if err != nil {
		return nil, nil, err
	}

	fsCtx := wasm.NewFSContext(preopens)
	return context.WithValue(ctx, FSKey{}, fsCtx), fsCtx, nil
}
