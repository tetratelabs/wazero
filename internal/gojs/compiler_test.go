package gojs_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/gojs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// Test_compileJsWasm ensures the infrastructure to generate wasm on-demand works.
func Test_compileJsWasm(t *testing.T) {
	bin := compileJsWasm(t, `package main

import "os"

func main() {
	os.Exit(1)
}`)

	m, err := binary.DecodeModule(bin, wasm.Features20191205, wasm.MemorySizer)
	require.NoError(t, err)
	// TODO: implement go.buildid custom section and validate it instead.
	require.NotNil(t, m.MemorySection)
}

func Test_compileAndRunJsWasm(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, `package main

func main() {}`, wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Zero(t, stdout)
}

func compileAndRunJsWasm(ctx context.Context, t *testing.T, goSrc string, config wazero.ModuleConfig) (stdout, stderr string, err error) {
	bin := compileJsWasm(t, goSrc)

	var stdoutBuf, stderrBuf bytes.Buffer

	r := wazero.NewRuntimeWithConfig(testCtx, wazero.NewRuntimeConfig().WithWasmCore2())
	defer r.Close(ctx)

	compiled, compileErr := r.CompileModule(ctx, bin, wazero.NewCompileConfig())
	if compileErr != nil {
		err = compileErr
		return
	}

	err = gojs.Run(ctx, r, compiled, config.WithStdout(&stdoutBuf).WithStderr(&stderrBuf))
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}

// compileJsWasm allows us to generate a binary with runtime.GOOS=js and runtime.GOARCH=wasm.
func compileJsWasm(t *testing.T, goSrc string) []byte {
	goBin, err := findGoBin()
	if err != nil {
		t.Skip("Skipping tests due to missing Go binary: ", err)
	}
	// For some reason, windows and freebsd fail to compile with exit status 1.
	if os := runtime.GOOS; os != "darwin" && os != "linux" {
		t.Skip("Skipping tests due to not yet supported OS: ", os)
	}

	workDir := t.TempDir()

	err = os.WriteFile(filepath.Join(workDir, "main.go"), []byte(goSrc), 0o600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(workDir, "go.mod"),
		[]byte("module github.com/tetratelabs/wazero/experimental/gojs\n\ngo 1.18\n"), 0o600)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bin := "out.wasm"
	cmd := exec.CommandContext(ctx, goBin, "build", "-o", bin, ".") //nolint:gosec
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "couldn't compile %s: %s", bin, string(out))

	binBytes, err := os.ReadFile(filepath.Join(workDir, bin)) //nolint:gosec
	require.NoError(t, err, "couldn't compile %s", bin)
	return binBytes
}

func findGoBin() (string, error) {
	binName := "go"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", binName)
	if _, err := os.Stat(goBin); err == nil {
		return goBin, nil
	}
	// Now, search the path
	return exec.LookPath(binName)
}
