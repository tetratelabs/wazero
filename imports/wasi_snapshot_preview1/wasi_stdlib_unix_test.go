//go:build unix || linux || darwin

package wasi_snapshot_preview1_test

import (
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_NonblockingFile(t *testing.T) {
	const fifo = "/test-fifo"
	tempDir := t.TempDir()
	fifoAbsPath := tempDir + fifo

	moduleConfig := wazero.NewModuleConfig().
		WithArgs("wasi", "nonblock", fifo).
		WithFSConfig(wazero.NewFSConfig().WithDirMount(tempDir, "/")).
		WithSysNanosleep()

	err := syscall.Mkfifo(fifoAbsPath, 0o666)
	require.NoError(t, err)

	ch := make(chan string, 1)
	go func() { ch <- compileAndRun(t, testCtx, moduleConfig, wasmZigCc) }()

	// The test writes a few dots on the console until the pipe has data ready
	// for reading. So, so we wait to ensure those dots are printed.
	sleepALittle()

	f, err := os.OpenFile(fifoAbsPath, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	n, err := f.Write([]byte("wazero"))
	require.NoError(t, err)
	require.NotEqual(t, 0, n)
	console := <-ch
	lines := strings.Split(console, "\n")

	// Check if the first line starts with at least one dot.
	require.True(t, strings.HasPrefix(lines[0], "."))
	require.Equal(t, "wazero", lines[1])
}
