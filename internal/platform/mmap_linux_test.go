package platform

import (
	"os"
	"testing"

	"github.com/tetratelabs/wazero/internal/features"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func init() {
	features.EnableFromEnvironment()
}

func TestHugePageConfigs(t *testing.T) {
	if !hasHugePages() {
		t.Skip("hugepages are disabled")
	}
	dirents, err := os.ReadDir("/sys/kernel/mm/hugepages/")
	require.NoError(t, err)
	require.Equal(t, len(dirents), len(hugePageConfigs))

	for _, hugePageConfig := range hugePageConfigs {
		require.NotEqual(t, 0, hugePageConfig.size)
		require.NotEqual(t, 0, hugePageConfig.flag)
	}

	for i := 1; i < len(hugePageConfigs); i++ {
		require.True(t, hugePageConfigs[i-1].size > hugePageConfigs[i].size)
	}
}
