package wasi_snapshot_preview1_test

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"io/fs"
	"net"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalnet "github.com/tetratelabs/wazero/experimental/net"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/fsapi"
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
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", "./a-"), bin)

		requireLsOut(t, "\n", expectDots, console)
	})

	t.Run("not a directory", func(t *testing.T) {
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", "-"), bin)

		require.Equal(t, `
ENOTDIR
`, "\n"+console)
	})

	t.Run("directory with entries", func(t *testing.T) {
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", "."), bin)
		requireLsOut(t, `
./-
./a-
./ab-
`, expectDots, console)
	})

	t.Run("directory with entries - read twice", func(t *testing.T) {
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", ".", "repeat"), bin)
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
		console := compileAndRun(t, testCtx, config, bin)

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

	console := compileAndRun(t, testCtx, moduleConfig.WithFS(fstest.MapFS{}), bin)

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

	console := compileAndRun(t, testCtx, moduleConfig.
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

func compileAndRun(t *testing.T, ctx context.Context, config wazero.ModuleConfig, bin []byte) (console string) {
	return compileAndRunWithPreStart(t, ctx, config, bin, nil)
}

func compileAndRunWithPreStart(t *testing.T, ctx context.Context, config wazero.ModuleConfig, bin []byte, preStart func(t *testing.T, mod api.Module)) (console string) {
	// same for console and stderr as sometimes the stack trace is in one or the other.
	var consoleBuf bytes.Buffer

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	require.NoError(t, err)

	mod, err := r.InstantiateWithConfig(ctx, bin, config.
		WithStdout(&consoleBuf).
		WithStderr(&consoleBuf).
		WithStartFunctions()) // clear
	require.NoError(t, err)

	if preStart != nil {
		preStart(t, mod)
	}

	_, err = mod.ExportedFunction("_start").Call(ctx)
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
		stdin           fsapi.File
		expectedOutput  string
		expectedTimeout time.Duration
	}{
		{
			name:            "custom reader, data ready, not tty",
			args:            []string{"wasi", "poll"},
			stdin:           &internalsys.StdinFile{Reader: strings.NewReader("test")},
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
		{
			name:            "custom reader, data ready, not tty, .5sec",
			args:            []string{"wasi", "poll", "0", "500"},
			stdin:           &internalsys.StdinFile{Reader: strings.NewReader("test")},
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
		{
			name:            "custom reader, data ready, tty, .5sec",
			args:            []string{"wasi", "poll", "0", "500"},
			stdin:           &ttyStdinFile{StdinFile: internalsys.StdinFile{Reader: strings.NewReader("test")}},
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
		{
			name:            "custom, blocking reader, no data, tty, .5sec",
			args:            []string{"wasi", "poll", "0", "500"},
			stdin:           &neverReadyTtyStdinFile{StdinFile: internalsys.StdinFile{Reader: newBlockingReader(t)}},
			expectedOutput:  "NOINPUT",
			expectedTimeout: 500 * time.Millisecond, // always timeouts
		},
		{
			name:            "eofReader, not tty, .5sec",
			args:            []string{"wasi", "poll", "0", "500"},
			stdin:           &ttyStdinFile{StdinFile: internalsys.StdinFile{Reader: eofReader{}}},
			expectedOutput:  "STDIN",
			expectedTimeout: 0 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			console := compileAndRunWithPreStart(t, testCtx, wazero.NewModuleConfig().WithArgs(tc.args...), wasmZigCc,
				func(t *testing.T, mod api.Module) {
					setStdin(t, mod, tc.stdin)
				})
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
	console := compileAndRun(t, testCtx, moduleConfig, wasmZigCc)
	require.True(t, time.Since(start) >= 100*time.Millisecond)
	require.Equal(t, "OK\n", console)
}

func Test_open(t *testing.T) {
	for toolchain, bin := range map[string][]byte{
		"zig-cc": wasmZigCc,
	} {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testOpenReadOnly(t, bin)
			testOpenWriteOnly(t, bin)
		})
	}
}

func testOpenReadOnly(t *testing.T, bin []byte) {
	testOpen(t, "rdonly", bin)
}

func testOpenWriteOnly(t *testing.T, bin []byte) {
	testOpen(t, "wronly", bin)
}

func testOpen(t *testing.T, cmd string, bin []byte) {
	t.Run(cmd, func(t *testing.T) {
		moduleConfig := wazero.NewModuleConfig().
			WithArgs("wasi", "open-"+cmd).
			WithFSConfig(wazero.NewFSConfig().WithDirMount(t.TempDir(), "/"))

		console := compileAndRun(t, testCtx, moduleConfig, bin)
		require.Equal(t, "OK", strings.TrimSpace(console))
	})
}

func Test_Sock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows is not supported yet")
	}

	moduleConfig := wazero.NewModuleConfig().WithArgs("wasi", "socket")
	// Instruct wazero to create the listener using the addr:port pair that was created and destroyed earlier.
	// We assume that nobody stole that port in the meantime.
	netCfg := experimentalnet.NewConfig().WithTCPListener("127.0.0.1", 0)
	ctx := experimentalnet.WithConfig(testCtx, netCfg)
	tcpAddrCh := make(chan *net.TCPAddr, 1)
	ch := make(chan string, 1)
	go func() {
		ch <- compileAndRunWithPreStart(t, ctx, moduleConfig, wasmZigCc, func(t *testing.T, mod api.Module) {
			tcpAddrCh <- requireTCPListenerAddr(t, mod)
		})
	}()
	tcpAddr := <-tcpAddrCh

	// Give a little time for _start to complete
	time.Sleep(800 * time.Millisecond)

	// Now dial to the initial address, which should be now held by wazero.
	conn, err := net.Dial("tcp", tcpAddr.String())
	require.NoError(t, err)
	defer conn.Close()

	n, err := conn.Write([]byte("wazero"))
	console := <-ch
	require.NotEqual(t, 0, n)
	require.NoError(t, err)
	require.Equal(t, "wazero\nOK\n", console)
}
