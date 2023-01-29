package gojs_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_goroutine(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "goroutine", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `producer
consumer
`, stdout)
}

func Test_mem(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "mem", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Zero(t, stdout)
}

func Test_stdio(t *testing.T) {
	t.Parallel()

	input := "stdin\n"
	stdout, stderr, err := compileAndRun(testCtx, "stdio", wazero.NewModuleConfig().
		WithStdin(strings.NewReader(input)))

	require.Equal(t, "stderr 6\n", stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "stdout 6\n", stdout)
}

func Test_stdio_large(t *testing.T) {
	t.Parallel()

	// Large stdio will trigger GC which will trigger events.
	var log bytes.Buffer
	loggingCtx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{},
		logging.NewHostLoggingListenerFactory(&log, logging.LogScopePoll))

	size := 2 * 1024 * 1024 // 2MB
	input := make([]byte, size)
	stdout, stderr, err := compileAndRun(loggingCtx, "stdio", wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(input)))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, fmt.Sprintf("stderr %d\n", size), stderr)
	require.Equal(t, fmt.Sprintf("stdout %d\n", size), stdout)

	// We can't predict the precise ms the timeout event will be, so we partial match.
	require.Contains(t, log.String(), `==> go.runtime.scheduleTimeoutEvent(ms=`)
	require.Contains(t, log.String(), `<== (id=1)`)
	// There may be another timeout event between the first and its clear.
	require.Contains(t, log.String(), `==> go.runtime.clearTimeoutEvent(id=1)
<==
`)
}

func Test_gc(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "gc", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, "", stderr)
	require.Equal(t, "before gc\nafter gc\n", stdout)
}
