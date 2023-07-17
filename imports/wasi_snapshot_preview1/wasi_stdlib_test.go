package wasi_snapshot_preview1_test

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
	gofstest "testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalsock "github.com/tetratelabs/wazero/experimental/sock"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/fstest"
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

// wasmGotip is conditionally compiled from testdata/gotip/wasi.go
var wasmGotip []byte

// wasmTinyGo was compiled from testdata/tinygo/wasi.go
//
//go:embed testdata/tinygo/wasi.wasm
var wasmTinyGo []byte

// wasmZigCc was compiled from testdata/zig-cc/wasi.c
//
//go:embed testdata/zig-cc/wasi.wasm
var wasmZigCc []byte

// wasmZig was compiled from testdata/zig/wasi.c
//
//go:embed testdata/zig/wasi.wasm
var wasmZig []byte

func Test_fdReaddir_ls(t *testing.T) {
	toolchains := map[string][]byte{
		"cargo-wasi": wasmCargoWasi,
		"tinygo":     wasmTinyGo,
		"zig-cc":     wasmZigCc,
		"zig":        wasmZig,
	}
	if wasmGotip != nil {
		toolchains["gotip"] = wasmGotip
	}

	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))

	tons := path.Join(tmpDir, "tons")
	require.NoError(t, os.Mkdir(tons, 0o0777))
	for i := 0; i < direntCountTons; i++ {
		require.NoError(t, os.WriteFile(path.Join(tons, strconv.Itoa(i)), nil, 0o0666))
	}

	for toolchain, bin := range toolchains {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			var expectDots int
			if toolchain == "zig-cc" {
				expectDots = 1
			}
			testFdReaddirLs(t, bin, toolchain, tmpDir, expectDots)
		})
	}
}

const direntCountTons = 8096

func testFdReaddirLs(t *testing.T, bin []byte, toolchain, rootDir string, expectDots int) {
	t.Helper()

	moduleConfig := wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().
			WithReadOnlyDirMount(path.Join(rootDir, "dir"), "/"))

	t.Run("empty directory", func(t *testing.T) {
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", "./a-"), bin)

		requireLsOut(t, nil, expectDots, console)
	})

	t.Run("not a directory", func(t *testing.T) {
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", "-"), bin)

		require.Equal(t, `
ENOTDIR
`, "\n"+console)
	})

	t.Run("directory with entries", func(t *testing.T) {
		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", "."), bin)
		requireLsOut(t, []string{
			"./-",
			"./a-",
			"./ab-",
		}, expectDots, console)
	})

	t.Run("directory with entries - read twice", func(t *testing.T) {
		if toolchain == "tinygo" {
			t.Skip("https://github.com/tinygo-org/tinygo/issues/3823")
		}

		console := compileAndRun(t, testCtx, moduleConfig.WithArgs("wasi", "ls", ".", "repeat"), bin)
		requireLsOut(t, []string{
			"./-",
			"./a-",
			"./ab-",
			"./-",
			"./a-",
			"./ab-",
		}, expectDots*2, console)
	})

	t.Run("directory with tons of entries", func(t *testing.T) {
		moduleConfig = wazero.NewModuleConfig().
			WithFSConfig(wazero.NewFSConfig().
				WithReadOnlyDirMount(path.Join(rootDir, "tons"), "/")).
			WithArgs("wasi", "ls", ".")

		console := compileAndRun(t, testCtx, moduleConfig, bin)

		lines := strings.Split(console, "\n")
		expected := direntCountTons + 1 /* trailing newline */
		expected += expectDots * 2
		require.Equal(t, expected, len(lines))
	})
}

func requireLsOut(t *testing.T, expected []string, expectDots int, console string) {
	for i := 0; i < expectDots; i++ {
		expected = append(expected, "./.", "./..")
	}

	actual := strings.Split(console, "\n")
	sort.Strings(actual) // os directories are not lexicographic order
	actual = actual[1:]  // trailing newline

	sort.Strings(expected)
	if len(actual) == 0 {
		require.Nil(t, expected)
	} else {
		require.Equal(t, expected, actual)
	}
}

func Test_fdReaddir_stat(t *testing.T) {
	toolchains := map[string][]byte{
		"cargo-wasi": wasmCargoWasi,
		"tinygo":     wasmTinyGo,
		"zig-cc":     wasmZigCc,
		"zig":        wasmZig,
	}
	if wasmGotip != nil {
		toolchains["gotip"] = wasmGotip
	}

	for toolchain, bin := range toolchains {
		toolchain := toolchain
		bin := bin
		t.Run(toolchain, func(t *testing.T) {
			testFdReaddirStat(t, bin)
		})
	}
}

func testFdReaddirStat(t *testing.T, bin []byte) {
	moduleConfig := wazero.NewModuleConfig().WithArgs("wasi", "stat")

	console := compileAndRun(t, testCtx, moduleConfig.WithFS(gofstest.MapFS{}), bin)

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
			WithFSMount(gofstest.MapFS{}, "/tmp")), bin)

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

// compileAndRunForked executes the test case with the wazero runtime in a separate process, and waits for it to terminate.
//
// Stdout is captured to a buffer, and stderr is dumped to os.Stderr.
// It returns the capture buffer and boolean; the boolean is true if we are running in the outer process.
// If it is false, it means we are running in the subprocess; all the verification logic should be handled
// when the boolean is true.
//
// A typical usage pattern will be:
//
//	if buf, hasRun := compileAndRunForked(...); hasRun {
//		validateContents(buf)
//	}
func compileAndRunForked(t *testing.T, ctx context.Context, config wazero.ModuleConfig, tname string, bin []byte) ([]byte, bool) {
	var buf bytes.Buffer
	// We use the technique described in https://go.dev/talks/2014/testing.slide#23
	// We check if we are running forked by ensuring that a "magic" environment variable is set.
	if os.Getenv("_TEST_FORKED") != "1" {
		// If said variable is not set, then we need to exec this same executable, specifying the name of test.
		// We could use t.Name(), but because t may be arbitrarily nested, it is better to get the name of the test
		// from a parameter.
		cmd := exec.Command(os.Args[0], "-test.run", tname)
		cmd.Stdout = &buf
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "_TEST_FORKED=1")
		err := cmd.Run()
		if e, ok := err.(*exec.ExitError); ok && !e.Success() {
			require.NoError(t, e, "The test quit with an error code: %v\n", e)
		}
		res := buf.Bytes()
		// This is a test, so in case of success, it will include the "PASS\n" string:
		// we remove that from the output, to return a clean stdout.
		return res[0 : len(res)-len("PASS\n")], true
	}

	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	_, err := wasi_snapshot_preview1.Instantiate(ctx, r)
	require.NoError(t, err)

	mod, err := r.InstantiateWithConfig(ctx, bin, config.
		WithStartFunctions()) // clear
	require.NoError(t, err)

	_, err = mod.ExportedFunction("_start").Call(ctx)
	if exitErr, ok := err.(*sys.ExitError); ok {
		require.Zero(t, exitErr.ExitCode())
	} else {
		require.NoError(t, err)
	}
	return nil, false
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
	// Nonblocking connections may contain error logging, we ignore those.
	require.Equal(t, "wazero\n", console[len(console)-7:])
}

func Test_HTTP(t *testing.T) {
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
	stdinReader, stdinWriter, err := os.Pipe()
	require.NoError(t, err)
	stdoutReader, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)
	defer func() {
		stdinReader.Close()
		stdinWriter.Close()
		stdoutReader.Close()
		stdoutReader.Close()
	}()
	require.NoError(t, err)
	moduleConfig := wazero.NewModuleConfig().
		WithSysNanotime(). // poll_oneoff requires nanotime.
		WithArgs("wasi", "stdin").
		WithStdin(stdinReader).
		WithStdout(stdoutWriter)
	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch)

		r := wazero.NewRuntime(testCtx)
		defer r.Close(testCtx)
		_, err := wasi_snapshot_preview1.Instantiate(testCtx, r)
		require.NoError(t, err)
		_, err = r.InstantiateWithConfig(testCtx, bin, moduleConfig)
		require.NoError(t, err)
	}()

	time.Sleep(1 * time.Second)
	buf := make([]byte, 21)
	_, _ = stdoutReader.Read(buf)
	require.Equal(t, "waiting for stdin...\n", string(buf))
	_, _ = stdinWriter.WriteString("foo")
	_ = stdinWriter.Close()
	buf = make([]byte, 3)
	_, _ = stdoutReader.Read(buf)
	require.Equal(t, "foo", string(buf))
	<-ch
}

func Test_LargeStdout(t *testing.T) {
	toolchains := map[string][]byte{}
	if wasmGotip != nil {
		toolchains["gotip"] = wasmGotip
	}

	for toolchain, bin := range toolchains {
		toolchain := toolchain
		bin := bin
		name := t.Name()
		t.Run(toolchain, func(t *testing.T) {
			testLargeStdout(t, name, bin)
		})
	}
}

func testLargeStdout(t *testing.T, tname string, bin []byte) {
	// This test dumps a large Go source file to stdout. The generated result
	// should be valid code, otherwise it means that stdout is corrupted.
	//
	// The error conditions are more easily reproduced by executing in a subprocess
	// and capturing its stdout.
	if buf, hasRun := compileAndRunForked(t, testCtx, wazero.NewModuleConfig().
		WithArgs("wasi", "largestdout").
		WithStdout(os.Stdout), tname, bin); hasRun {

		tempDir := t.TempDir()
		temp, err := os.Create(joinPath(tempDir, "out.go"))
		require.NoError(t, err)
		defer temp.Close()

		require.NoError(t, err)
		_, _ = temp.Write(buf)
		_ = temp.Close()

		gotipBin, err := findGotipBin()
		require.NoError(t, err)

		cmd := exec.CommandContext(testCtx, gotipBin, "build", "-o",
			joinPath(tempDir, "outbin"), temp.Name())
		require.NoError(t, err)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	}
}
