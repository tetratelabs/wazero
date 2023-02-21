package wasi_snapshot_preview1_test

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

func Test_schedYield(t *testing.T) {
	var yielded bool
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithOsyield(func() {
			yielded = true
		}))
	defer r.Close(testCtx)
	requireErrnoResult(t, ErrnoSuccess, mod, SchedYieldName)
	require.Equal(t, `
==> wasi_snapshot_preview1.sched_yield()
<== errno=ESUCCESS
`, "\n"+log.String())
	require.True(t, yielded)
}
