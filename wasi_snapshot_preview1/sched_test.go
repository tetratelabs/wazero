package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_schedYield only tests it is stubbed for GrainLang per #271
func Test_schedYield(t *testing.T) {
	log := requireErrnoNosys(t, functionSchedYield)
	require.Equal(t, `
--> wasi_snapshot_preview1.sched_yield()
<-- ENOSYS
`, log)
}
