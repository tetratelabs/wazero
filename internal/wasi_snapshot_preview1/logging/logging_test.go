package logging

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type testFunctionDefinition struct {
	name string
	*wasm.FunctionDefinition
}

// Name implements the same method as documented on api.FunctionDefinition.
func (f *testFunctionDefinition) Name() string {
	return f.name
}

func TestIsInLogScope(t *testing.T) {
	randomGet := &testFunctionDefinition{name: RandomGetName}
	fdRead := &testFunctionDefinition{name: FdReadName}
	tests := []struct {
		name     string
		fnd      api.FunctionDefinition
		scopes   logging.LogScopes
		expected bool
	}{
		{
			name:     "randomGet in LogScopeCrypto",
			fnd:      randomGet,
			scopes:   logging.LogScopeCrypto,
			expected: true,
		},
		{
			name:     "randomGet not in LogScopeFilesystem",
			fnd:      randomGet,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "randomGet in LogScopeCrypto|LogScopeFilesystem",
			fnd:      randomGet,
			scopes:   logging.LogScopeCrypto | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "randomGet in LogScopeNone",
			fnd:      randomGet,
			scopes:   logging.LogScopeNone,
			expected: true,
		},
		{
			name:     "fdRead in LogScopeFilesystem",
			fnd:      fdRead,
			scopes:   logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "fdRead not in LogScopeCrypto",
			fnd:      fdRead,
			scopes:   logging.LogScopeCrypto,
			expected: false,
		},
		{
			name:     "fdRead in LogScopeCrypto|LogScopeFilesystem",
			fnd:      fdRead,
			scopes:   logging.LogScopeCrypto | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "fdRead in LogScopeNone",
			fnd:      fdRead,
			scopes:   logging.LogScopeNone,
			expected: true,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsInLogScope(tc.fnd, tc.scopes))
		})
	}
}
