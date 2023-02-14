package platform

import (
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_archRequirementsVerified(t *testing.T) {
	switch runtime.GOARCH {
	case "arm64":
		require.True(t, archRequirementsVerified)
	case "amd64":
		// TODO: once we find a way to test no SSE4 platform, use build tag and choose the correct assertion.
		// For now, we assume that all the amd64 machine we are testing are with SSE 4 to avoid
		// accidentally turn off compiler on the modern amd64 platform.
		require.True(t, archRequirementsVerified)
	default:
		require.False(t, archRequirementsVerified)
	}
}
