package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_main ensures the following will work:
//
//	go run cat.go /test.txt
func Test_main(t *testing.T) {
	for _, toolchain := range []string{"cargo-wasi", "tinygo", "zig", "zig-cc"} {
		toolchain := toolchain
		t.Run(toolchain, func(t *testing.T) {
			t.Setenv("TOOLCHAIN", toolchain)
			stdout, stderr := maintester.TestMain(t, main, "cat", "test.txt")
			require.Equal(t, "", stderr)
			require.Equal(t, "greet filesystem\n", stdout)
		})
	}
}

// Test_cli ensures the following will work:
//
// go run github.com/tetratelabs/wazero/cmd/wazero run -mount=testdata:/ cat.wasm /test.txt
func Test_cli(t *testing.T) {
	tests := []struct {
		toolchain string
		wasm      []byte
	}{
		{
			toolchain: "cargo-wasi",
			wasm:      catWasmCargoWasi,
		},
		{
			toolchain: "tinygo",
			wasm:      catWasmTinyGo,
		},
		{
			toolchain: "zig",
			wasm:      catWasmZig,
		},
		{
			toolchain: "zig-cc",
			wasm:      catWasmZigCc,
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.toolchain, func(t *testing.T) {
			for _, testPath := range []string{"test.txt", "testcases/test.txt"} {
				if tt.toolchain == "zig" && testPath == "testcases/test.txt" {
					// Zig only resolves absolute paths under the first
					// pre-open (cwd), so it won't find this file until #1077
					continue
				}
				t.Run(testPath, func(t *testing.T) {
					// Write out embedded files instead of accessing directly for docker cross-architecture tests.
					wasmPath := filepath.Join(t.TempDir(), "cat.wasm")
					require.NoError(t, os.WriteFile(wasmPath, tt.wasm, 0o755))

					testTxt, err := fs.ReadFile(catFS, "testdata/test.txt")
					require.NoError(t, err)
					testTxtPath := filepath.Join(t.TempDir(), "test.txt")
					require.NoError(t, os.WriteFile(testTxtPath, testTxt, 0o755))

					// We can't invoke go run in our docker based cross-architecture tests. We do want to use
					// otherwise so running unit tests normally does not require special build steps.
					var cmdExe string
					var cmdArgs []string
					if cmdPath := os.Getenv("WAZEROCLI"); cmdPath != "" {
						cmdExe = cmdPath
					} else {
						cmdExe = filepath.Join(runtime.GOROOT(), "bin", "go")
						cmdArgs = []string{"run", "../../../cmd/wazero"}
					}

					cmdArgs = append(cmdArgs, "run",
						"-hostlogging=filesystem",
						fmt.Sprintf("-mount=%s:/", filepath.Dir(testTxtPath)),
						fmt.Sprintf("-mount=%s:/testcases", filepath.Dir(testTxtPath)),
						wasmPath, testPath)

					var stdOut, stdErr bytes.Buffer
					cmd := exec.Command(cmdExe, cmdArgs...)
					cmd.Stdout = &stdOut
					cmd.Stderr = &stdErr
					require.NoError(t, cmd.Run(), stdErr.String())
					require.Equal(t, "greet filesystem\n", stdOut.String())
				})
			}
		})
	}
}
