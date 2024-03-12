package wasi_snapshot_preview1_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
)

func TestMain(m *testing.M) {
	var err error
	if wasmGo, err = compileWasip1Wasm(); err != nil {
		println("go: skipping due compilation error:", err)
	}
	os.Exit(m.Run())
}

// runtimeCfg is a shared runtime configuration for tests within this package to reduce the compilation time of the binary.
var runtimeCfg = wazero.NewRuntimeConfig().WithCompilationCache(wazero.NewCompilationCache())

// compileWasip1Wasm allows us to generate a binary with runtime.GOOS=wasip1
// and runtime.GOARCH=wasm. This intentionally does so on-demand, because the
// wasm is too big to check in.
func compileWasip1Wasm() ([]byte, error) {
	// Prepare the working directory.
	workdir, err := os.MkdirTemp("", "wasi")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workdir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binPath := path.Join(workdir, "wasi.wasm")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".") //nolint:gosec
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	cmd.Dir = "testdata/go"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("couldn't compile %s: %w", string(out), err)
	}

	bin, err := os.ReadFile(binPath) //nolint:gosec
	if err != nil {
		return nil, err
	}

	// Proactively compile the binary to reduce the test time.
	r := wazero.NewRuntimeWithConfig(context.Background(), runtimeCfg)
	defer r.Close(context.Background())
	_, err = r.CompileModule(context.Background(), bin)
	return bin, err
}
