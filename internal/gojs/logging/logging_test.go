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
	runtimeResetMemoryDataView := &testFunctionDefinition{name: custom.NameRuntimeResetMemoryDataView}
	runtimeWasmExit := &testFunctionDefinition{name: custom.NameRuntimeWasmExit}
	syscallValueCall := &testFunctionDefinition{name: custom.NameSyscallValueCall}
	tests := []struct {
		name     string
		fnd      api.FunctionDefinition
		scopes   logging.LogScopes
		expected bool
	}{
		{
			name:     "runtimeWasmExit in LogScopeProc",
			fnd:      runtimeWasmExit,
			scopes:   logging.LogScopeProc,
			expected: true,
		},
		{
			name:     "runtimeWasmExit not in LogScopeFilesystem",
			fnd:      runtimeWasmExit,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "runtimeWasmExit in LogScopeProc|LogScopeFilesystem",
			fnd:      runtimeWasmExit,
			scopes:   logging.LogScopeProc | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "runtimeWasmExit not in LogScopeNone",
			fnd:      runtimeWasmExit,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "runtimeWasmExit in LogScopeAll",
			fnd:      runtimeWasmExit,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "runtimeResetMemoryDataView in LogScopeMemory",
			fnd:      runtimeResetMemoryDataView,
			scopes:   logging.LogScopeMemory,
			expected: true,
		},
		{
			name:     "runtimeResetMemoryDataView not in LogScopeFilesystem",
			fnd:      runtimeResetMemoryDataView,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "runtimeResetMemoryDataView in LogScopeMemory|LogScopeFilesystem",
			fnd:      runtimeResetMemoryDataView,
			scopes:   logging.LogScopeMemory | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "runtimeResetMemoryDataView not in LogScopeNone",
			fnd:      runtimeResetMemoryDataView,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "runtimeResetMemoryDataView in LogScopeAll",
			fnd:      runtimeResetMemoryDataView,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "runtimeGetRandomData not in LogScopeFilesystem",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "runtimeGetRandomData in LogScopeRandom|LogScopeFilesystem",
			fnd:      runtimeGetRandomData,
			scopes:   logging.LogScopeRandom | logging.LogScopeFilesystem,
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
			name:     "syscallValueCall in LogScopeRandom",
			fnd:      syscallValueCall,
			scopes:   logging.LogScopeRandom,
			expected: true,
		},
		{
			name:     "syscallValueCall in LogScopeRandom|LogScopeFilesystem",
			fnd:      syscallValueCall,
			scopes:   logging.LogScopeRandom | logging.LogScopeFilesystem,
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
