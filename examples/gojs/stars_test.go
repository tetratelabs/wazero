package main

import (
	"fmt"
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
	// Notably our scratch containers don't have go, so don't fail tests.
	if err := compileWasm(); err != nil {
		t.Skip("Skipping tests due to:", err)
	}

	stdout, stderr := maintester.TestMain(t, main, "stars")
	require.Equal(t, "", stderr)
	require.Equal(t, "wazero has 9999999 stars. Does that include you?\n", stdout)
}

// compileWasm compiles "stars/main.go" on demand as the binary generated is
// too big (>1MB) to check into the source tree.
func compileWasm() error {
	cmd := exec.Command("go", "build", "-o", "main.wasm", ".")
	cmd.Dir = path.Join("stars")

	cmd.Env = append(os.Environ(), "GOARCH=wasm", "GOOS=js")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	return nil
}
