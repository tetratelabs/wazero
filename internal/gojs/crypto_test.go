package gojs_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_crypto(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	loggingCtx := context.WithValue(testCtx, experimental.FunctionListenerFactoryKey{}, logging.NewScopedLoggingListenerFactory(&log, logging.LogScopeCrypto))

	stdout, stderr, err := compileAndRun(loggingCtx, "crypto", wazero.NewModuleConfig())

	require.Zero(t, stderr)
	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Equal(t, `7a0c9f9f0d
`, stdout)
	require.Equal(t, `==> go.runtime.getRandomData(r_len=32)
<==
==> go.runtime.getRandomData(r_len=8)
<==
`, log.String())
}
