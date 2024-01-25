package platform

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAmd64CpuId_cpuHasFeature(t *testing.T) {
	flags := cpuFeatureFlags{
		flags:      uint64(CpuFeatureAmd64SSE3),
		extraFlags: uint64(CpuExtraFeatureAmd64ABM),
	}
	require.True(t, flags.Has(CpuFeatureAmd64SSE3))
	require.False(t, flags.Has(CpuFeatureAmd64SSE4_2))
	require.True(t, flags.HasExtra(CpuExtraFeatureAmd64ABM))
	require.False(t, flags.HasExtra(1<<6)) // some other value
}
