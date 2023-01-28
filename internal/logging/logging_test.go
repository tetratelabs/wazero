package logging

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestLogScopes tests the bitset works as expected
func TestLogScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes LogScopes
	}{
		{
			name:   "one is the smallest flag",
			scopes: 1,
		},
		{
			name:   "63 is the largest feature flag", // because uint64
			scopes: 1 << 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := LogScopes(0)

			// Defaults to false
			require.False(t, f.IsEnabled(tc.scopes))

			// Set true makes it true
			f = f | tc.scopes
			require.True(t, f.IsEnabled(tc.scopes))

			// Set false makes it false again
			f = f ^ tc.scopes
			require.False(t, f.IsEnabled(tc.scopes))
		})
	}
}

func TestLogScopes_String(t *testing.T) {
	tests := []struct {
		name     string
		scopes   LogScopes
		expected string
	}{
		{name: "none", scopes: LogScopeNone, expected: ""},
		{name: "any", scopes: LogScopeAll, expected: "all"},
		{name: "filesystem", scopes: LogScopeFilesystem, expected: "filesystem"},
		{name: "crypto", scopes: LogScopeCrypto, expected: "crypto"},
		{name: "filesystem|crypto", scopes: LogScopeFilesystem | LogScopeCrypto, expected: "filesystem|crypto"},
		{name: "undefined", scopes: 1 << 3, expected: fmt.Sprintf("<unknown=%d>", (1 << 3))},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.scopes.String())
		})
	}
}
