package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/wasi_arg.wasm
var wasmWasiArg []byte

//go:embed testdata/wasi_env.wasm
var wasmWasiEnv []byte

//go:embed testdata/wasi_fd.wasm
var wasmWasiFd []byte

//go:embed testdata/fs/bear.txt
var bearTxt []byte

func TestRun(t *testing.T) {
	bearPath := filepath.Join(t.TempDir(), "bear.txt")
	require.NoError(t, os.WriteFile(bearPath, bearTxt, 0o755))

	tests := []struct {
		name       string
		wazeroOpts []string
		wasm       []byte
		wasmArgs   []string
		stdOut     string
		stdErr     string
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
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.name, func(t *testing.T) {
			wasmPath := filepath.Join(t.TempDir(), "test.wasm")
			require.NoError(t, os.WriteFile(wasmPath, tt.wasm, 0o755))

			args := append([]string{"run"}, tt.wazeroOpts...)
			args = append(args, wasmPath)
			args = append(args, tt.wasmArgs...)
			exitCode, stdOut, stdErr := runMain(t, args)
			require.Equal(t, 0, exitCode, stdErr)
			require.Equal(t, tt.stdOut, stdOut)
			require.Equal(t, tt.stdErr, stdErr)
		})
	}
}

func TestHelp(t *testing.T) {
	exitCode, _, stdErr := runMain(t, []string{"-h"})
	require.Equal(t, 0, exitCode)
	require.Contains(t, stdErr, "wazero CLI\n\nUsage:")
}

func TestErrors(t *testing.T) {
	notWasmPath := filepath.Join(t.TempDir(), "bears.wasm")
	require.NoError(t, os.WriteFile(notWasmPath, []byte("pooh"), 0o755))

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
