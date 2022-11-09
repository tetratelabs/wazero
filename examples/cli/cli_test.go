package cli

import (
	"bytes"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/cli.wasm
var cliWasm []byte

func TestRun(t *testing.T) {
	tests := []struct {
		args   []string
		stdOut string
	}{
		{
			args:   []string{"3", "1"},
			stdOut: "result: 4",
		},
		{
			args:   []string{"-sub=true", "3", "1"},
			stdOut: "result: 2",
		},
	}

	wasmPath := filepath.Join(t.TempDir(), "cli.wasm")
	require.NoError(t, os.WriteFile(wasmPath, cliWasm, 0o755))

	// We can't invoke go run in our docker based cross-architecture tests. We do want to use
	// otherwise so running unit tests normally does not require special build steps.
	var cmdExe string
	var cmdArgs []string
	if cmdPath := os.Getenv("WAZEROCLI"); cmdPath != "" {
		cmdExe = cmdPath
	} else {
		cmdExe = filepath.Join(runtime.GOROOT(), "bin", "go")
		cmdArgs = []string{"run", "../../cmd/wazero"}
	}
	cmdArgs = append(cmdArgs, "run", wasmPath)

	for _, tc := range tests {
		tt := tc
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			stdOut := &bytes.Buffer{}
			cmd := exec.Command(cmdExe, append(cmdArgs, tt.args...)...)
			cmd.Stdout = stdOut
			require.NoError(t, cmd.Run())
			require.Equal(t, tt.stdOut, strings.TrimSpace(stdOut.String()))
		})
	}
}
