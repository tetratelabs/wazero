package main

import (
	"bytes"
	"embed"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata
var testdata embed.FS

func TestRun(t *testing.T) {
	tests := []struct {
		wasmPath string
		wasmArgs []string
		stdOut   string
		stdErr   string
	}{
		{
			wasmPath: "testdata/wasi_arg.wasm",
			wasmArgs: []string{"hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
		},
		{
			wasmPath: "testdata/wasi_arg.wasm",
			wasmArgs: []string{"--", "hello world"},
			// Executable name is first arg so is printed.
			stdOut: "test.wasm\x00hello world\x00",
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.wasmPath, func(t *testing.T) {
			wasmBytes, err := fs.ReadFile(testdata, tt.wasmPath)
			require.NoError(t, err)

			wasmPath := filepath.Join(t.TempDir(), "test.wasm")
			require.NoError(t, os.WriteFile(wasmPath, wasmBytes, 0755))

			exitCode, stdOut, stdErr := runMain(t, append([]string{"run", wasmPath}, tt.wasmArgs...))
			require.Equal(t, 0, exitCode)
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
