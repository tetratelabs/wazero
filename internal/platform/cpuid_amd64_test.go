package platform

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestAmd64CpuId_cpuHasFeature(t *testing.T) {
	flags := cpuFeatureFlags{
		flags:      CpuFeatureSSE3,
		extraFlags: CpuExtraFeatureABM,
	}
	require.True(t, flags.Has(CpuFeatureSSE3))
	require.False(t, flags.Has(CpuFeatureSSE4_2))
	require.True(t, flags.HasExtra(CpuExtraFeatureABM))
	require.False(t, flags.HasExtra(1<<6)) // some other value
}
