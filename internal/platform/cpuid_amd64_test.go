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

func TestAmd64CpuFeatureFlags_Raw(t *testing.T) {
	flags := cpuFeatureFlags{
		flags:      uint64(CpuFeatureAmd64SSE3 | CpuFeatureAmd64SSE4_1 | CpuFeatureAmd64SSE4_2),
		extraFlags: uint64(CpuExtraFeatureAmd64ABM),
	}
	require.Equal(t, uint64(0b1111), flags.Raw())
	flags.flags = 0
	require.Equal(t, uint64(0b1000), flags.Raw())
	flags.extraFlags = 0
	require.Equal(t, uint64(0), flags.Raw())
}
