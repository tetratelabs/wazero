package gojs_test

import (
	"os"
	"path"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_fs(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "fs", wazero.NewModuleConfig().WithFS(testFS))

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, `wd ok
Not a directory
sub mode drwxr-xr-x
/animals.txt mode -rw-r--r--
animals.txt mode -rw-r--r--
contents: bear
cat
shark
dinosaur
human

empty:
`, stdout)
}

// Test_testsfs runs fstest.TestFS inside wasm.
func Test_testfs(t *testing.T) {
	t.Parallel()

	// Setup /testfs which is used in the wasm invocation of testfs.TestFS.
	tmpDir := t.TempDir()
	testfsDir := path.Join(tmpDir, "testfs")
	require.NoError(t, os.Mkdir(testfsDir, 0o700))
	require.NoError(t, fstest.WriteTestFiles(testfsDir))

	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	stdout, stderr, err := compileAndRun(testCtx, "testfs", wazero.NewModuleConfig().WithFSConfig(fsConfig))

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stdout)
}

func Test_writefs(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")

	// test expects to write under /tmp
	require.NoError(t, os.Mkdir(path.Join(tmpDir, "tmp"), 0o700))

	stdout, stderr, err := compileAndRun(testCtx, "writefs", wazero.NewModuleConfig().WithFSConfig(fsConfig))

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)

	if platform.CompilerSupported() {
		//  Note: as of Go 1.19, only the Sec field is set on update in fs_js.go.
		require.Equal(t, `/tmp/dir mode drwx------
/tmp/dir/file mode -rw-------
dir times: 123000000000 567000000000
`, stdout)
	} else { // only mtimes will return on a plarform we don't support in sysfs
		require.Equal(t, `/tmp/dir mode drwx------
/tmp/dir/file mode -rw-------
dir times: 567000000000 567000000000
`, stdout)
	}
}
