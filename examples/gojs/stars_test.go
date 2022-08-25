package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
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
	if err := compileWasm(); err != nil {
		log.Println("Skipping tests due to:", err)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// compileWasm compiles "stars/main.go" on demand as the binary generated is
// too big (>7MB) to check into the source tree.
func compileWasm() error {
	cmd := exec.Command("go", "build", "-o", "main.wasm", ".")
	cmd.Dir = path.Join("stars")

	cmd.Env = append(os.Environ(), "GOARCH=wasm", "GOOS=js")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
}
