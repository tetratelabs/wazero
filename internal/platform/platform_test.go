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

func Test_isAtLeastGo120(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{input: "go1.18.10", expected: false},
		{input: "go1.19.10", expected: false},
		{input: "go1.20.5", expected: true},
		{input: "devel go1.21-39c50707 Thu Jul 6 23:23:41 2023 +0000", expected: true},
		{input: "go1.21rc2", expected: true},
		{input: "go1.90.10", expected: true},
		{input: "go2.0.0", expected: false},
	}

	for _, tt := range tests {
		tc := tt

		require.Equal(t, tc.expected, isAtLeastGo120(tc.input), tc.input)
	}
}
