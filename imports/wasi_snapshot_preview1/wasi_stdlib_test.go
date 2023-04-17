package wasi_snapshot_preview1_test

import (
	"bufio"
	"bytes"
	_ "embed"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// This file ensures that the behavior we've implemented not only the wasi
// spec, but also at least two compilers use of sdks.

// wasmCargoWasi was compiled from testdata/cargo-wasi/wasi.rs
//
//go:embed testdata/cargo-wasi/wasi.wasm
var wasmCargoWasi []byte

// wasmZigCc was compiled from testdata/zig-cc/wasi.c
//
//go:embed testdata/zig-cc/wasi.wasm
var wasmZigCc []byte

// wasmZig was compiled from testdata/zig/wasi.c
//
//go:embed testdata/zig/wasi.wasm
var wasmZig []byte

func Test_fdReaddir_ls(t *testing.T) {
	for toolchain, bin := range map[string][]byte{
		"cargo-wasi": wasmCargoWasi,
		"zig-cc":     wasmZigCc,
		"zig":        wasmZig,
	} {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			expectDots := toolchain == "zig-cc"
			testFdReaddirLs(t, bin, expectDots)
		})
	}
}

func testFdReaddirLs(t *testing.T, bin []byte, expectDots bool) {
	// TODO: make a subfs
	moduleConfig := wazero.NewModuleConfig().
		WithFS(fstest.MapFS{
			"-":   {},
			"a-":  {Mode: fs.ModeDir},
			"ab-": {},
		})

	t.Run("empty directory", func(t *testing.T) {
		console := compileAndRun(t, moduleConfig.WithArgs("wasi", "ls", "./a-"), bin)

		requireLsOut(t, "\n", expectDots, console)
	})

	t.Run("not a directory", func(t *testing.T) {
		console := compileAndRun(t, moduleConfig.WithArgs("wasi", "ls", "-"), bin)

		require.Equal(t, `
ENOTDIR
`, "\n"+console)
	})

	t.Run("directory with entries", func(t *testing.T) {
		console := compileAndRun(t, moduleConfig.WithArgs("wasi", "ls", "."), bin)
		requireLsOut(t, `
./-
./a-
./ab-
`, expectDots, console)
	})

	t.Run("directory with entries - read twice", func(t *testing.T) {
		console := compileAndRun(t, moduleConfig.WithArgs("wasi", "ls", ".", "repeat"), bin)
		if expectDots {
			require.Equal(t, `
./.
./..
./-
./a-
./ab-
./.
./..
./-
./a-
./ab-
`, "\n"+console)
		} else {
			require.Equal(t, `
./-
./a-
./ab-
./-
./a-
./ab-
`, "\n"+console)
		}
	})

	t.Run("directory with tons of entries", func(t *testing.T) {
		testFS := fstest.MapFS{}
		count := 8096
		for i := 0; i < count; i++ {
			testFS[strconv.Itoa(i)] = &fstest.MapFile{}
		}
		config := wazero.NewModuleConfig().WithFS(testFS).WithArgs("wasi", "ls", ".")
		console := compileAndRun(t, config, bin)

		lines := strings.Split(console, "\n")
		expected := count + 1 /* trailing newline */
		if expectDots {
			expected += 2
		}
		require.Equal(t, expected, len(lines))
	})
}

func requireLsOut(t *testing.T, expected string, expectDots bool, console string) {
	dots := `
./.
./..
`
	if expectDots {
		expected = dots + expected[1:]
	}
	require.Equal(t, expected, "\n"+console)
}

func Test_fdReaddir_stat(t *testing.T) {
	for toolchain, bin := range map[string][]byte{
		"cargo-wasi": wasmCargoWasi,
		"zig-cc":     wasmZigCc,
		"zig":        wasmZig,
	} {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testFdReaddirStat(t, bin)
		})
	}
}

func testFdReaddirStat(t *testing.T, bin []byte) {
	moduleConfig := wazero.NewModuleConfig().WithArgs("wasi", "stat")

	console := compileAndRun(t, moduleConfig.WithFS(fstest.MapFS{}), bin)

	// TODO: switch this to a real stat test
	require.Equal(t, `
stdin isatty: false
stdout isatty: false
stderr isatty: false
/ isatty: false
`, "\n"+console)
}

func Test_preopen(t *testing.T) {
	for toolchain, bin := range map[string][]byte{
		"zig": wasmZig,
	} {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testPreopen(t, bin)
		})
	}
}

func testPreopen(t *testing.T, bin []byte) {
	moduleConfig := wazero.NewModuleConfig().WithArgs("wasi", "preopen")

	console := compileAndRun(t, moduleConfig.
		WithFSConfig(wazero.NewFSConfig().
			WithDirMount(".", "/").
			WithFSMount(fstest.MapFS{}, "/tmp")), bin)

	require.Equal(t, `
0: stdin
1: stdout
2: stderr
3: /
4: /tmp
`, "\n"+console)
}

func compileAndRun(t *testing.T, config wazero.ModuleConfig, bin []byte) (console string) {
	// same for console and stderr as sometimes the stack trace is in one or the other.
	var consoleBuf bytes.Buffer

	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	_, err := wasi_snapshot_preview1.Instantiate(testCtx, r)
	require.NoError(t, err)

	_, err = r.InstantiateWithConfig(testCtx, bin,
		config.WithStdout(&consoleBuf).WithStderr(&consoleBuf))
	if exitErr, ok := err.(*sys.ExitError); ok {
		require.Zero(t, exitErr.ExitCode(), consoleBuf.String())
	} else {
		require.NoError(t, err, consoleBuf.String())
	}

	console = consoleBuf.String()
	return
}

func Test_Poll(t *testing.T) {
	// The following test cases replace Stdin with a custom reader.
	// For more precise coverage, see poll_test.go.

	tests := []struct {
		name            string
		args            []string
		stdin           io.Reader
		expectedOutput  string
		expectedTimeout time.Duration
	}{
		{
			name: "custom reader, data ready, not tty",
			args: []string{"wasi", "poll"},
			stdin: internalsys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")), // input ready
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640),
				internalsys.PollerAlwaysReady),
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
		{
			name: "custom reader, data ready, not tty, .5sec",
			args: []string{"wasi", "poll", "0", "500"},
			stdin: internalsys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")), // input ready
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640),
				internalsys.PollerAlwaysReady),
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
		{
			name: "custom reader, data ready, tty, .5sec",
			args: []string{"wasi", "poll", "0", "500"},
			stdin: internalsys.NewStdioFileReader(
				bufio.NewReader(strings.NewReader("test")), // input ready
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640),
				internalsys.PollerAlwaysReady),
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
		{
			name: "custom, blocking reader, no data, tty, .5sec",
			args: []string{"wasi", "poll", "0", "500"},
			stdin: internalsys.NewStdioFileReader(
				bufio.NewReader(newBlockingReader(t)), // simulate waiting for input
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640),
				internalsys.PollerNeverReady),
			expectedOutput:  "NOINPUT",
			expectedTimeout: 500 * time.Millisecond, // always timeouts
		},
		{
			name: "eofReader, not tty, .5sec",
			args: []string{"wasi", "poll", "0", "500"},
			stdin: internalsys.NewStdioFileReader(
				bufio.NewReader(eofReader{}), // simulate waiting for input
				stdinFileInfo(fs.ModeDevice|fs.ModeCharDevice|0o640),
				internalsys.PollerAlwaysReady),
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			moduleConfig := wazero.NewModuleConfig().WithArgs(tc.args...).
				WithStdin(tc.stdin)
			start := time.Now()
			console := compileAndRun(t, moduleConfig, wasmZigCc)
			elapsed := time.Since(start)
			require.True(t, elapsed >= tc.expectedTimeout)
			require.Equal(t, tc.expectedOutput+"\n", console)
		})
	}
}

// eofReader is safer than reading from os.DevNull as it can never overrun operating system file descriptors.
type eofReader struct{}

// Read implements io.Reader
// Note: This doesn't use a pointer reference as it has no state and an empty struct doesn't allocate.
func (eofReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

func Test_Sleep(t *testing.T) {
	moduleConfig := wazero.NewModuleConfig().WithArgs("wasi", "sleepmillis", "100").WithSysNanosleep()
	start := time.Now()
	console := compileAndRun(t, moduleConfig, wasmZigCc)
	require.True(t, time.Since(start) >= 100*time.Millisecond)
	require.Equal(t, "OK\n", console)
}
