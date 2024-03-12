//go:build unix

package wasi_snapshot_preview1_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
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

	ch := make(chan string, 2)
	go func() {
		ch <- compileAndRunWithPreStart(t, testCtx, moduleConfig, wasmZigCc, func(t *testing.T, mod api.Module) {
			// Send a dummy string to signal that initialization has completed.
			ch <- "ready"
		})
	}()

	// Wait for the dummy value, then start the sleep.
	require.Equal(t, "ready", <-ch)

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

type fifo struct {
	file *os.File
	path string
}

func Test_NonblockGo(t *testing.T) {
	// - Create `numFifos` FIFOs.
	// - Instantiate `wasmGo` with the names of the FIFO in the order of creation
	// - The test binary opens the FIFOs in the given order and spawns a goroutine for each
	// - The unit test writes to the FIFO in reverse order.
	// - Each goroutine reads from the given FIFO and writes the contents to stderr
	//
	// The test verifies that the output order matches the write order (i.e. reverse order).
	//
	// If I/O was blocking, all goroutines would be blocked waiting for one read call
	// to return, and the output order wouldn't match.
	//
	// Adapted from https://github.com/golang/go/blob/0fcc70ecd56e3b5c214ddaee4065ea1139ae16b5/src/runtime/internal/wasitest/nonblock_test.go

	if wasmGo == nil {
		t.Skip("skipping because wasi.go was not compiled (go missing or compilation error)")
	}
	const numFifos = 8

	for _, mode := range []string{"open", "create"} {
		t.Run(mode, func(t *testing.T) {
			tempDir := t.TempDir()

			args := []string{"wasi", "nonblock", mode}
			fifos := make([]*fifo, numFifos)
			for i := range fifos {
				tempFile := fmt.Sprintf("wasip1-nonblock-fifo-%d-%d", rand.Uint32(), i)
				path := filepath.Join(tempDir, tempFile)
				err := syscall.Mkfifo(path, 0o666)
				require.NoError(t, err)

				file, err := os.OpenFile(path, os.O_RDWR, 0)
				require.NoError(t, err)
				defer file.Close()

				args = append(args, tempFile)
				fifos[len(fifos)-i-1] = &fifo{file, path}
			}

			pr, pw := io.Pipe()
			defer pw.Close()

			var consoleBuf bytes.Buffer

			moduleConfig := wazero.NewModuleConfig().
				WithArgs(args...).
				WithFSConfig( // Mount the tempDir as root.
						wazero.NewFSConfig().WithDirMount(tempDir, "/")).
				WithStderr(pw). // Write Stderr to pw
				WithStdout(&consoleBuf).
				WithStartFunctions().
				WithSysNanosleep()

			ch := make(chan string, 1)
			go func() {
				r := wazero.NewRuntimeWithConfig(testCtx, runtimeCfg)
				defer func() {
					require.NoError(t, r.Close(testCtx))
				}()

				_, err := wasi_snapshot_preview1.Instantiate(testCtx, r)
				require.NoError(t, err)

				compiled, err := r.CompileModule(testCtx, wasmGo)
				require.NoError(t, err)

				mod, err := r.InstantiateModule(testCtx, compiled, moduleConfig)
				require.NoError(t, err)

				_, err = mod.ExportedFunction("_start").Call(testCtx)
				if exitErr, ok := err.(*sys.ExitError); ok {
					require.Zero(t, exitErr.ExitCode(), consoleBuf.String())
				}
				ch <- consoleBuf.String()
			}()

			scanner := bufio.NewScanner(pr)
			require.True(t, scanner.Scan(), fmt.Sprintf("expected line: %s", scanner.Err()))
			require.Equal(t, "waiting", scanner.Text(), fmt.Sprintf("unexpected output: %s", scanner.Text()))

			for _, fifo := range fifos {
				_, err := fifo.file.WriteString(fifo.path + "\n")
				require.NoError(t, err)
				require.True(t, scanner.Scan(), fmt.Sprintf("expected line: %s", scanner.Err()))
				require.Equal(t, fifo.path, scanner.Text(), fmt.Sprintf("unexpected line: %s", scanner.Text()))
			}

			s := <-ch
			require.Equal(t, "", s)
		})
	}
}
