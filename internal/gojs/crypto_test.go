package gojs_test

import (
	"bytes"
	"context"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_crypto(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	loggingCtx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{},
		logging.NewHostLoggingListenerFactory(&log, logging.LogScopeCrypto))

	stdout, stderr, err := compileAndRun(loggingCtx, "crypto", wazero.NewModuleConfig())

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, `7a0c9f9f0d
`, stdout)

	// TODO: Go 1.17 initializes randoms in a different order than Go 1.18,19
	// When we move to 1.20, remove the workaround.
	info, ok := debug.ReadBuildInfo()
	require.True(t, ok)
	if strings.HasPrefix(info.GoVersion, "go1.17") {
		require.Equal(t, `==> go.runtime.getRandomData(r_len=8)
<==
==> go.runtime.getRandomData(r_len=32)
<==
==> go.syscall/js.valueCall(crypto.getRandomValues(r_len=5))
<== (n=5)
`, log.String())
	} else {
		require.Equal(t, `==> go.runtime.getRandomData(r_len=32)
<==
==> go.runtime.getRandomData(r_len=8)
<==
==> go.syscall/js.valueCall(crypto.getRandomValues(r_len=5))
<== (n=5)
`, log.String())
	}
}
