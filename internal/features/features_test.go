package features_test

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/features"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func init() {
	features.Enable("hugepages")
}

func TestList(t *testing.T) {
	require.Equal(t, []string{"hugepages"}, features.List())
}

func TestEnabled(t *testing.T) {
	require.True(t, features.Have("hugepages"))
	require.False(t, features.Have("nope"))
}

func TestAllocsEnabled(t *testing.T) {
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		features.Have("hugepages")
	}))
}

func TestAllocsDisabled(t *testing.T) {
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		features.Have("nope")
	}))
}
