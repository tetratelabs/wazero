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

	require.Zero(t, stderr)
	require.NoError(t, err)
	require.Equal(t, `Local
1ms
`, stdout)

	// To avoid multiple similar assertions, just check three functions we
	// expect were called.
	logString := logString(log)
	require.Contains(t, logString, `==> go.runtime.nanotime1()
<== (nsec=0)`)
	require.Contains(t, logString, `==> go.runtime.walltime()
<== (sec=1640995200,nsec=0)
`)
	require.Contains(t, logString, `==> go.syscall/js.valueCall(Date.getTimezoneOffset())
<== (tz=0)
`)
}
