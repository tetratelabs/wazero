package gojs_test

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_fs(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "fs", wazero.NewModuleConfig().WithFS(testFS))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `TestFS ok
wd ok
Not a directory
/test.txt ok
test.txt ok
contents: animals

empty: 
`, stdout)
}
