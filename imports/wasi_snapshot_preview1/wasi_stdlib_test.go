package wasi_snapshot_preview1

import (
	"bytes"
	_ "embed"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// lsZigCc was compiled from testdata/zig/ls.zig
//
//go:embed testdata/zig/ls.wasm
var lsZig []byte

// lsZigCc was compiled from testdata/zig-cc/ls.c
//
//go:embed testdata/zig-cc/ls.wasm
var lsZigCc []byte

// Test_fdReaddir_e2e ensures that the behavior we've implemented not only
// matches the wasi spec, but also at least two compilers use of sdks.
func Test_fdReaddir_ls(t *testing.T) {
	for toolchain, bin := range map[string][]byte{
		"zig":    lsZig,
		"zig-cc": lsZigCc,
	} {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testFdReaddirLs(t, bin)
		})
	}
}

func testFdReaddirLs(t *testing.T, bin []byte) {
	t.Run("empty directory", func(t *testing.T) {
		stdout, stderr := compileAndRun(t, wazero.NewModuleConfig().
			WithFS(fstest.MapFS{}), bin)

		require.Zero(t, stderr)
		require.Zero(t, stdout)
	})

	t.Run("directory with entries", func(t *testing.T) {
		stdout, stderr := compileAndRun(t, wazero.NewModuleConfig().
			WithFS(fstest.MapFS{
				"-":   {},
				"a-":  {Mode: fs.ModeDir},
				"ab-": {},
			}), bin)

		require.Zero(t, stderr)
		require.Equal(t, `-
a-
ab-
`, stdout)
	})

	t.Run("directory with tons of entries", func(t *testing.T) {
		testFS := fstest.MapFS{}
		count := 8096
		for i := 0; i < count; i++ {
			testFS[strconv.Itoa(i)] = &fstest.MapFile{}
		}
		stdout, stderr := compileAndRun(t, wazero.NewModuleConfig().
			WithFS(testFS), bin)

		require.Zero(t, stderr)
		lines := strings.Split(stdout, "\n")
		require.Equal(t, count+1 /* trailing newline */, len(lines))
	})
}

func compileAndRun(t *testing.T, config wazero.ModuleConfig, bin []byte) (stdout, stderr string) {
	var stdoutBuf, stderrBuf bytes.Buffer

	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	_, err := Instantiate(testCtx, r)
	require.NoError(t, err)

	compiled, err := r.CompileModule(testCtx, bin)
	require.NoError(t, err)

	_, err = r.InstantiateModule(testCtx, compiled, config.WithStdout(&stdoutBuf).WithStderr(&stderrBuf))
	if exitErr, ok := err.(*sys.ExitError); ok {
		require.Zero(t, exitErr.ExitCode())
	} else {
		require.NoError(t, err)
	}

	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}
