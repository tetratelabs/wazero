package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
)

//go:embed testdata/wasi_arg.wasm
var wasmWasiArg []byte

//go:embed testdata/wasi_env.wasm
var wasmWasiEnv []byte

//go:embed testdata/wasi_fd.wasm
var wasmWasiFd []byte

//go:embed testdata/fs/bear.txt
var bearTxt []byte

func TestMain(m *testing.M) {
	// For some reason, riscv64 fails to see directory listings.
	if a := runtime.GOARCH; a == "riscv64" {
		log.Println("gojs: skipping due to not yet supported GOARCH:", a)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCompile(t *testing.T) {
	tmpDir, oldwd := requireChdirToTemp(t)
	defer os.Chdir(oldwd) //nolint

	wasmPath := filepath.Join(tmpDir, "test.wasm")
	require.NoError(t, os.WriteFile(wasmPath, wasmWasiArg, 0o600))

	existingDir1 := filepath.Join(tmpDir, "existing1")
	require.NoError(t, os.Mkdir(existingDir1, 0o700))
	existingDir2 := filepath.Join(tmpDir, "existing2")
	require.NoError(t, os.Mkdir(existingDir2, 0o700))

	tests := []struct {
		name       string
		wazeroOpts []string
		test       func(t *testing.T)
	}{
		{
			name: "no opts",
		},
		{
			name:       "cachedir existing absolute",
			wazeroOpts: []string{"--cachedir=" + existingDir1},
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir1)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir existing relative",
			wazeroOpts: []string{"--cachedir=existing2"},
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir2)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new absolute",
			wazeroOpts: []string{"--cachedir=" + path.Join(tmpDir, "new1")},
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new1")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new relative",
			wazeroOpts: []string{"--cachedir=new2"},
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new2")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"compile"}, tt.wazeroOpts...)
			args = append(args, wasmPath)
			exitCode, stdOut, stdErr := runMain(t, args)
			require.Zero(t, stdErr)
			require.Equal(t, 0, exitCode, stdErr)
			require.Zero(t, stdOut)
			if test := tt.test; test != nil {
				test(t)
			}
		})
	}
}

func requireChdirToTemp(t *testing.T) (string, string) {
	tmpDir := t.TempDir()
	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	return tmpDir, oldwd
}

func TestCompile_Errors(t *testing.T) {
	tmpDir := t.TempDir()

	wasmPath := filepath.Join(tmpDir, "test.wasm")
	require.NoError(t, os.WriteFile(wasmPath, wasmWasiArg, 0o600))

	notWasmPath := filepath.Join(tmpDir, "bears.wasm")
	require.NoError(t, os.WriteFile(notWasmPath, []byte("pooh"), 0o600))

	tests := []struct {
		message string
		args    []string
	}{
		{
			message: "missing path to wasm file",
			args:    []string{},
		},
		{
			message: "error reading wasm binary",
			args:    []string{"non-existent.wasm"},
		},
		{
			message: "error compiling wasm binary",
			args:    []string{notWasmPath},
		},
		{
			message: "invalid cachedir",
			args:    []string{"--cachedir", notWasmPath, wasmPath},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.message, func(t *testing.T) {
			exitCode, _, stdErr := runMain(t, append([]string{"compile"}, tt.args...))

			require.Equal(t, 1, exitCode)
			require.Contains(t, stdErr, tt.message)
		})
	}
}

func TestRun(t *testing.T) {
	tmpDir, oldwd := requireChdirToTemp(t)
	defer os.Chdir(oldwd) //nolint

	bearPath := filepath.Join(tmpDir, "bear.txt")
	require.NoError(t, os.WriteFile(bearPath, bearTxt, 0o600))

	existingDir1 := filepath.Join(tmpDir, "existing1")
	require.NoError(t, os.Mkdir(existingDir1, 0o700))
	existingDir2 := filepath.Join(tmpDir, "existing2")
	require.NoError(t, os.Mkdir(existingDir2, 0o700))

	tests := []struct {
		name       string
		wazeroOpts []string
		wasm       []byte
		wasmArgs   []string
		stdOut     string
		stdErr     string
		test       func(t *testing.T)
	}{
		{
			name:     "args",
			wasm:     wasmWasiArg,
			wasmArgs: []string{"hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
		},
		{
			name:     "-- args",
			wasm:     wasmWasiArg,
			wasmArgs: []string{"--", "hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
		},
		{
			name:       "env",
			wasm:       wasmWasiEnv,
			wazeroOpts: []string{"--env=ANIMAL=bear", "--env=FOOD=sushi"},
			stdOut:     "ANIMAL=bear\x00FOOD=sushi\x00",
		},
		{
			name:       "fd",
			wasm:       wasmWasiFd,
			wazeroOpts: []string{fmt.Sprintf("--mount=%s:/", filepath.Dir(bearPath))},
			stdOut:     "pooh\n",
		},
		{
			name:       "cachedir existing absolute",
			wazeroOpts: []string{"--cachedir=" + existingDir1},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir1)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir existing relative",
			wazeroOpts: []string{"--cachedir=existing2"},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir(existingDir2)
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new absolute",
			wazeroOpts: []string{"--cachedir=" + path.Join(tmpDir, "new1")},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new1")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
		{
			name:       "cachedir new relative",
			wazeroOpts: []string{"--cachedir=new2"},
			wasm:       wasmWasiArg,
			wasmArgs:   []string{"hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
			test: func(t *testing.T) {
				entries, err := os.ReadDir("new2")
				require.NoError(t, err)
				require.True(t, len(entries) > 0)
			},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.name, func(t *testing.T) {
			wasmPath := filepath.Join(tmpDir, "test.wasm")
			require.NoError(t, os.WriteFile(wasmPath, tt.wasm, 0o700))

			args := append([]string{"run"}, tt.wazeroOpts...)
			args = append(args, wasmPath)
			args = append(args, tt.wasmArgs...)
			exitCode, stdOut, stdErr := runMain(t, args)
			require.Equal(t, 0, exitCode, stdErr)
			require.Equal(t, tt.stdOut, stdOut)
			require.Equal(t, tt.stdErr, stdErr)
			if test := tt.test; test != nil {
				test(t)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	exitCode, stdOut, stdErr := runMain(t, []string{"version"})
	require.Equal(t, 0, exitCode)
	require.Equal(t, version.GetCommitHash()+"\n", stdOut)
	require.Equal(t, "", stdErr)
}

func TestRun_Errors(t *testing.T) {
	wasmPath := filepath.Join(t.TempDir(), "test.wasm")
	require.NoError(t, os.WriteFile(wasmPath, wasmWasiArg, 0o700))

	notWasmPath := filepath.Join(t.TempDir(), "bears.wasm")
	require.NoError(t, os.WriteFile(notWasmPath, []byte("pooh"), 0o700))

	tests := []struct {
		message string
		args    []string
	}{
		{
			message: "missing path to wasm file",
			args:    []string{},
		},
		{
			message: "error reading wasm binary",
			args:    []string{"non-existent.wasm"},
		},
		{
			message: "error compiling wasm binary",
			args:    []string{notWasmPath},
		},
		{
			message: "invalid environment variable",
			args:    []string{"--env=ANIMAL", "testdata/wasi_env.wasm"},
		},
		{
			message: "invalid mount",
			args:    []string{"--mount=.", "testdata/wasi_env.wasm"},
		},
		{
			message: "invalid cachedir",
			args:    []string{"--cachedir", notWasmPath, wasmPath},
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.message, func(t *testing.T) {
			exitCode, _, stdErr := runMain(t, append([]string{"run"}, tt.args...))

			require.Equal(t, 1, exitCode)
			require.Contains(t, stdErr, tt.message)
		})
	}
}

func TestHelp(t *testing.T) {
	exitCode, _, stdErr := runMain(t, []string{"-h"})
	require.Equal(t, 0, exitCode)
	require.Contains(t, stdErr, "wazero CLI\n\nUsage:")
}

func runMain(t *testing.T, args []string) (int, string, string) {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() {
		os.Args = oldArgs
	})
	os.Args = append([]string{"wazero"}, args...)

	var exitCode int
	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}
	var exited bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				exited = true
			}
		}()
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		doMain(stdOut, stdErr, func(code int) {
			exitCode = code
			panic(code)
		})
	}()

	require.True(t, exited)

	return exitCode, stdOut.String(), stdErr.String()
}
