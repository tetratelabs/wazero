package logging

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
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
	runtimeGetRandomData := &testFunctionDefinition{name: custom.NameRuntimeGetRandomData}
	syscallValueCall := &testFunctionDefinition{name: custom.NameSyscallValueCall}
	tests := []struct {
		name     string
		fnd      api.FunctionDefinition
		scopes   logging.LogScopes
		expected bool
	}{
		{
			name:     "runtimeGetRandomData in LogScopeCrypto",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeCrypto,
			expected: true,
		},
		{
			name:     "runtimeGetRandomData not in LogScopeFilesystem",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "runtimeGetRandomData in LogScopeCrypto|LogScopeFilesystem",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeCrypto | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "runtimeGetRandomData not in LogScopeNone",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "runtimeGetRandomData in LogScopeAll",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "syscallValueCall in LogScopeFilesystem",
			fnd:      syscallValueCall,
			scopes:   logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "syscallValueCall not in LogScopeCrypto",
			fnd:      syscallValueCall,
			scopes:   logging.LogScopeCrypto,
			expected: false,
		},
		{
			name:     "syscallValueCall in LogScopeCrypto|LogScopeFilesystem",
			fnd:      syscallValueCall,
			scopes:   logging.LogScopeCrypto | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "syscallValueCall in LogScopeAll",
			fnd:      syscallValueCall,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "syscallValueCall not in LogScopeNone",
			fnd:      syscallValueCall,
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
