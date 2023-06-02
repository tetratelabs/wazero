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
		{name: "clock", scopes: LogScopeClock, expected: "clock"},
		{name: "proc", scopes: LogScopeProc, expected: "proc"},
		{name: "filesystem", scopes: LogScopeFilesystem, expected: "filesystem"},
		{name: "poll", scopes: LogScopePoll, expected: "poll"},
		{name: "random", scopes: LogScopeRandom, expected: "random"},
		{name: "sock", scopes: LogScopeSock, expected: "sock"},
		{name: "filesystem|random", scopes: LogScopeFilesystem | LogScopeRandom, expected: "filesystem|random"},
		{name: "undefined", scopes: 1 << 14, expected: fmt.Sprintf("<unknown=%d>", 1<<14)},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.scopes.String())
		})
	}
}
