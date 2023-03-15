package gojs_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_time(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	loggingCtx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{},
		logging.NewHostLoggingListenerFactory(&log, logging.LogScopeClock))

	stdout, stderr, err := compileAndRun(loggingCtx, "time", defaultConfig)

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `Local
1ms
`, stdout)

	// To avoid multiple similar assertions, just check three functions we
	// expect were called.
	require.Contains(t, log.String(), `==> go.runtime.nanotime1()
<== (nsec=0)`)
	require.Contains(t, log.String(), `==> go.runtime.walltime()
<== (sec=1640995200,nsec=0)
`)
	require.Contains(t, log.String(), `==> go.syscall/js.valueCall(Date.getTimezoneOffset())
<== (tz=0)
`)
}
