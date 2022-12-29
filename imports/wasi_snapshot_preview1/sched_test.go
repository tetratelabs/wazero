package wasi_snapshot_preview1_test

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

// Test_schedYield only tests it is stubbed for GrainLang per #271
func Test_schedYield(t *testing.T) {
	log := requireErrnoNosys(t, SchedYieldName)
	require.Equal(t, `
--> wasi_snapshot_preview1.sched_yield()
<-- errno=ENOSYS
`, log)
}
