package features_test

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/features"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func init() {
	features.Enable("hugepages")
}

func TestEnabled(t *testing.T) {
	require.True(t, features.Have(features.HugePages))
	require.False(t, features.Have(1<<31))
}

func TestAllocsEnabled(t *testing.T) {
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		features.Have(features.HugePages)
	}))
}

func TestAllocsDisabled(t *testing.T) {
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		features.Have(features.HugePages)
	}))
}
