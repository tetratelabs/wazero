package wasi_snapshot_preview1_test

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalsock "github.com/tetratelabs/wazero/experimental/sock"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/fsapi"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// sleepALittle directly slows down test execution. So, use this sparingly and
// only when so where proper signals are unavailable.
var sleepALittle = func() { time.Sleep(500 * time.Millisecond) }

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

// wasmGotip is conditionally compiled from testdata/gotip/wasi.go
var wasmGotip []byte

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

func Test_Open(t *testing.T) {
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

func Test_Hang(t *testing.T) {
	var consoleBuf bytes.Buffer
	ctx, cancel := context.WithCancel(testCtx)
	r, w := io.Pipe()

	rt := wazero.NewRuntime(ctx)

	_, err := wasi_snapshot_preview1.Instantiate(ctx, rt)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		_, err := rt.InstantiateWithConfig(ctx, wasmZigCc,
			wazero.NewModuleConfig().
				WithArgs("wasi", "echo").
				WithStdout(&consoleBuf).
				WithStderr(&consoleBuf).
				WithStdin(r))

		require.NoError(t, err)

		close(done)
	}()

	time.Sleep(2 * time.Second)
	_, _ = w.Write([]byte("test\n"))
	cancel()
	_ = r.Close()
	_ = rt.Close(context.Background())

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Errorf("should not timeout")
	}
}

func Test_Sock(t *testing.T) {
	toolchains := map[string][]byte{
		"cargo-wasi": wasmCargoWasi,
		"zig-cc":     wasmZigCc,
	}
	if wasmGotip != nil {
		toolchains["gotip"] = wasmGotip
	}

	for toolchain, bin := range toolchains {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testSock(t, bin)
		})
	}
}

func testSock(t *testing.T, bin []byte) {
	sockCfg := experimentalsock.NewConfig().WithTCPListener("127.0.0.1", 0)
	ctx := experimentalsock.WithConfig(testCtx, sockCfg)
	moduleConfig := wazero.NewModuleConfig().WithArgs("wasi", "sock")
	tcpAddrCh := make(chan *net.TCPAddr, 1)
	ch := make(chan string, 1)
	go func() {
		ch <- compileAndRunWithPreStart(t, ctx, moduleConfig, bin, func(t *testing.T, mod api.Module) {
			tcpAddrCh <- requireTCPListenerAddr(t, mod)
		})
	}()
	tcpAddr := <-tcpAddrCh

	// Give a little time for _start to complete
	sleepALittle()

	// Now dial to the initial address, which should be now held by wazero.
	conn, err := net.Dial("tcp", tcpAddr.String())
	require.NoError(t, err)
	defer conn.Close()

	n, err := conn.Write([]byte("wazero"))
	console := <-ch
	require.NotEqual(t, 0, n)
	require.NoError(t, err)
	require.Equal(t, "wazero\n", console)
}

func Test_HTTP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("syscall.Nonblocking() is not supported on wasip1+windows.")
	}
	toolchains := map[string][]byte{}
	if wasmGotip != nil {
		toolchains["gotip"] = wasmGotip
	}

	for toolchain, bin := range toolchains {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testHTTP(t, bin)
		})
	}
}

func testHTTP(t *testing.T, bin []byte) {
	sockCfg := experimentalsock.NewConfig().WithTCPListener("127.0.0.1", 0)
	ctx := experimentalsock.WithConfig(testCtx, sockCfg)

	moduleConfig := wazero.NewModuleConfig().
		WithSysWalltime().WithSysNanotime(). // HTTP middleware uses both clocks
		WithArgs("wasi", "http")
	tcpAddrCh := make(chan *net.TCPAddr, 1)
	ch := make(chan string, 1)
	go func() {
		ch <- compileAndRunWithPreStart(t, ctx, moduleConfig, bin, func(t *testing.T, mod api.Module) {
			tcpAddrCh <- requireTCPListenerAddr(t, mod)
		})
	}()
	tcpAddr := <-tcpAddrCh

	// Give a little time for _start to complete
	sleepALittle()

	// Now, send a POST to the address which we had pre-opened.
	body := bytes.NewReader([]byte("wazero"))
	req, err := http.NewRequest(http.MethodPost, "http://"+tcpAddr.String(), body)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "wazero\n", string(b))

	console := <-ch
	require.Equal(t, "", console)
}

func Test_Stdin(t *testing.T) {
	toolchains := map[string][]byte{}
	if wasmGotip != nil {
		toolchains["gotip"] = wasmGotip
	}

	for toolchain, bin := range toolchains {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testStdin(t, bin)
		})
	}
}

func testStdin(t *testing.T, bin []byte) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	moduleConfig := wazero.NewModuleConfig().
		WithSysNanotime(). // poll_oneoff requires nanotime.
		WithArgs("wasi", "stdin").
		WithStdin(r)
	ch := make(chan string, 1)
	go func() {
		ch <- compileAndRun(t, testCtx, moduleConfig, bin)
	}()
	time.Sleep(1 * time.Second)
	_, _ = w.WriteString("foo")
	s := <-ch
	require.Equal(t, "waiting for stdin...\nfoo", s)
}
