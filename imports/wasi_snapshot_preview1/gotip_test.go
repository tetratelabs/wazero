package wasi_snapshot_preview1_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// Find the gotip binary (if present), and compile the Wasm binary.
	if gotipBin, err := findGotipBin(); err != nil {
		println("gotip: skipping due missing binary:", err)
	} else if wasmGotip, err = compileWasip1Wasm(gotipBin); err != nil {
		println("gotip: skipping due compilation error:", err)
	}
	os.Exit(m.Run())
}

// compileWasip1Wasm allows us to generate a binary with runtime.GOOS=wasip1
// and runtime.GOARCH=wasm. This intentionally does so on-demand, because the
// wasm is too big to check in. Pls, not everyone will have gotip.
func compileWasip1Wasm(gotipBin string) ([]byte, error) {
	// Prepare the working directory.
	workdir, err := os.MkdirTemp("", "wasi")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workdir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bin := path.Join(workdir, "wasi.wasm")
	cmd := exec.CommandContext(ctx, gotipBin, "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	cmd.Dir = "testdata/gotip"
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("couldn't compile %s: %w", string(out), err)
	}

	return os.ReadFile(bin)
}

func findGotipBin() (string, error) {
	binName := "gotip"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	gotipBin := filepath.Join(runtime.GOROOT(), "bin", binName)
	if _, err := os.Stat(gotipBin); err == nil {
		return gotipBin, nil
	}
	// Now, search the path
	return exec.LookPath(binName)
}
