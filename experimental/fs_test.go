package experimental_test

import (
	"context"
	_ "embed"
	"log"
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
	if err != nil {
		log.Panicln(err)
	}
	defer closer.Close(ctx)

	v := ctx.Value(sys.FSKey{})
	require.NotNil(t, v)
	fsCtx, ok := v.(*sys.FSContext)
	require.True(t, ok)

	entry, ok := fsCtx.OpenedFile(3)
	require.True(t, ok)
	require.Equal(t, "/", entry.Path)
	require.Equal(t, mapfs, entry.FS)

	entry, ok = fsCtx.OpenedFile(4)
	require.True(t, ok)
	require.Equal(t, ".", entry.Path)
	require.Equal(t, mapfs, entry.FS)
}
