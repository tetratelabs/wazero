package features_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/features"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func init() {
	os.Setenv(features.EnvVarName, "f0,f1,f2")
}

func TestList(t *testing.T) {
	require.Equal(t, []string{"f0", "f1", "f2"}, features.List())
}

func TestEnabled(t *testing.T) {
	require.True(t, features.Enabled("f0"))
	require.True(t, features.Enabled("f1"))
	require.True(t, features.Enabled("f2"))
	require.False(t, features.Enabled("nope"))
}

func TestAllocsEnabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("accessing features allocates memory on windows")
	}
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		features.Enabled("f2")
	}))
}

func TestAllocsDisabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("accessing features allocates memory on windows")
	}
	require.Equal(t, 0.0, testing.AllocsPerRun(100, func() {
		features.Enabled("nope")
	}))
}
