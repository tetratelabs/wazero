package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/tetratelabs/wazero"
	gojs "github.com/tetratelabs/wazero/imports/go"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// Test_main ensures the following will work:
//
//	go run stars.go
func Test_main(t *testing.T) {
	stdout, stderr := maintester.TestMain(t, main, "stars")
	require.Equal(t, "", stderr)
	require.Equal(t, "wazero has 9999999 stars. Does that include you?\n", stdout)
}

// TestMain compiles the wasm on-demand, which uses the runner's Go as opposed
// to a binary checked in, which would be pinned to one version. This is
// separate from Test_main to show that compilation doesn't dominate the
// execution time.
func TestMain(m *testing.M) {
	// Notably our scratch containers don't have go, so don't fail tests.
	if err := compileFromGo(); err != nil {
		log.Println("Skipping tests due to:", err)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// compileFromGo compiles "stars/main.go" on demand as the binary generated is
// too big (>7MB) to check into the source tree.
func compileFromGo() error {
	cmd := exec.Command("go", "build", "-o", "main.wasm", ".")
	cmd.Dir = "stars"
	cmd.Env = append(os.Environ(), "GOARCH=wasm", "GOOS=js", "GOWASM=satconv,signext")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
}

// Benchmark_main is in the example for GOOS=js to re-use compilation caching
// infrastructure. This is only used to sporadically check the impact of
// internal changes as in general, it is known that GOOS=js will be slow due to
// JavaScript emulation.
func Benchmark_main(b *testing.B) {
	// Don't benchmark with interpreter as we know it will be slow.
	if !platform.CompilerSupported() {
		b.Skip()
	}

	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx) // This closes everything this Runtime created.

	bin, err := os.ReadFile(path.Join("stars", "main.wasm"))
	if err != nil {
		b.Fatal(err)
	}
	compiled, err := r.CompileModule(ctx, bin)
	if err != nil {
		b.Fatal(err)
	}

	// Instead of making real HTTP calls, return fake data.
	ctx = gojs.WithRoundTripper(ctx, &fakeGitHub{})
	cfg := wazero.NewModuleConfig()

	b.Run("gojs.Run", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			err = gojs.Run(ctx, r, compiled, cfg)
			if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
				b.Fatal(err)
			} else if !ok {
				b.Fatal(err)
			}
		}
	})
}
