package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func Test_procExit(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
	defer r.Close(testCtx)

	tests := []struct {
		name        string
		exitCode    uint32
		expectedLog string
	}{
		{
			name:     "success (exitcode 0)",
			exitCode: 0,
			expectedLog: `
==> wasi_snapshot_preview1.proc_exit(rval=0)
`,
		},
		{
			name:     "arbitrary non-zero exitcode",
			exitCode: 42,
			expectedLog: `
==> wasi_snapshot_preview1.proc_exit(rval=42)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Since procExit panics, any opcodes afterwards cannot be reached.
			_, err := mod.ExportedFunction(procExitName).Call(testCtx, uint64(tc.exitCode))
			require.Error(t, err)
			sysErr, ok := err.(*sys.ExitError)
			require.True(t, ok, err)
			require.Equal(t, tc.exitCode, sysErr.ExitCode())
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_procRaise only tests it is stubbed for GrainLang per #271
func Test_procRaise(t *testing.T) {
	log := requireErrnoNosys(t, procRaiseName, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.proc_raise(sig=0)
<-- errno=ENOSYS
`, log)
}
