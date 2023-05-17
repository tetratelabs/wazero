package platform

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestHugePageConfigs(t *testing.T) {
	dirents, err := os.ReadDir("/sys/kernel/mm/hugepages/")
	require.NoError(t, err)
	require.Equal(t, len(dirents), len(hugePagesConfigs))

	for _, hugePagesConfig := range hugePagesConfigs {
		require.NotEqual(t, 0, hugePagesConfig.size)
		require.NotEqual(t, 0, hugePagesConfig.flag)
	}

	for i := 1; i < len(hugePagesConfigs); i++ {
		require.True(t, hugePagesConfigs[i-1].size > hugePagesConfigs[i].size)
	}
}
