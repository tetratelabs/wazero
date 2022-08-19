package main

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
)

// TestGetWazeroVersion ensures that GetWazeroVersion returns the version of wazero in the go.mod in the
// downstream wazero users.
func TestGetWazeroVersion(t *testing.T) {
	// This matches the one in the "replace" statement in the go.mod.
	const exp = "v0.0.0-20220818123113-1948909ec0b1"
	require.Equal(t, exp, version.GetWazeroVersion())
}
