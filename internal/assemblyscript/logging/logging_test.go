package logging

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/internal/assemblyscript"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type testFunctionDefinition struct {
	name string
	*wasm.FunctionDefinition
}

// ExportNames implements the same method as documented on api.FunctionDefinition.
func (f *testFunctionDefinition) ExportNames() []string {
	return []string{f.name}
}

func TestIsInLogScope(t *testing.T) {
	abort := &testFunctionDefinition{name: AbortName}
	seed := &testFunctionDefinition{name: SeedName}
	tests := []struct {
		name     string
		fnd      api.FunctionDefinition
		scopes   logging.LogScopes
		expected bool
	}{
		{
			name:     "abort in LogScopeProc",
			fnd:      abort,
			scopes:   logging.LogScopeProc,
			expected: true,
		},
		{
			name:     "abort not in LogScopeFilesystem",
			fnd:      abort,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "abort in LogScopeProc|LogScopeFilesystem",
			fnd:      abort,
			scopes:   logging.LogScopeProc | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "abort in LogScopeAll",
			fnd:      abort,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "abort not in LogScopeNone",
			fnd:      abort,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "seed not in LogScopeFilesystem",
			fnd:      seed,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "seed in LogScopeRandom|LogScopeFilesystem",
			fnd:      seed,
			scopes:   logging.LogScopeRandom | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "seed in LogScopeAll",
			fnd:      seed,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "seed not in LogScopeNone",
			fnd:      seed,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsInLogScope(tc.fnd, tc.scopes))
		})
	}
}
