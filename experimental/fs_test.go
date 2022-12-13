package experimental_test

import (
	"context"
	_ "embed"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// This is a very basic integration of fs config. The main goal is to show how it is configured.
func TestWithFS(t *testing.T) {
	fileName := "animals.txt"
	mapfs := fstest.MapFS{fileName: &fstest.MapFile{Data: []byte(`animals`)}}

	// Set context to one that has experimental fs config
	ctx, closer, err := experimental.WithFS(context.Background(), mapfs)
	require.NoError(t, err)
	defer closer.Close(ctx)

	v := ctx.Value(sys.FSKey{})
	require.NotNil(t, v)
	fsCtx, ok := v.(*sys.FSContext)
	require.True(t, ok)

	entry, ok := fsCtx.OpenedFile(ctx, 3)
	require.True(t, ok)
	require.Equal(t, "/", entry.Name)

	// Override to nil context, ex to block file access
	ctx, closer, err = experimental.WithFS(ctx, nil)
	require.NoError(t, err)
	defer closer.Close(ctx)

	v = ctx.Value(sys.FSKey{})
	require.NotNil(t, v)
	fsCtx, ok = v.(*sys.FSContext)
	require.True(t, ok)

	_, ok = fsCtx.OpenedFile(ctx, 3)
	require.False(t, ok)
}
