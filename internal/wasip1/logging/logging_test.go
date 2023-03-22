package logging

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasip1"
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
	clockTimeGet := &testFunctionDefinition{name: ClockTimeGetName}
	fdRead := &testFunctionDefinition{name: FdReadName}
	pollOneoff := &testFunctionDefinition{name: PollOneoffName}
	procExit := &testFunctionDefinition{name: ProcExitName}
	randomGet := &testFunctionDefinition{name: RandomGetName}
	tests := []struct {
		name     string
		fnd      api.FunctionDefinition
		scopes   logging.LogScopes
		expected bool
	}{
		{
			name:     "clockTimeGet in LogScopeClock",
			fnd:      clockTimeGet,
			scopes:   logging.LogScopeClock,
			expected: true,
		},
		{
			name:     "clockTimeGet not in LogScopeFilesystem",
			fnd:      clockTimeGet,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "clockTimeGet in LogScopeClock|LogScopeFilesystem",
			fnd:      clockTimeGet,
			scopes:   logging.LogScopeClock | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "clockTimeGet in LogScopeAll",
			fnd:      clockTimeGet,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "clockTimeGet not in LogScopeNone",
			fnd:      clockTimeGet,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "fdRead in LogScopeFilesystem",
			fnd:      fdRead,
			scopes:   logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "fdRead not in LogScopeRandom",
			fnd:      fdRead,
			scopes:   logging.LogScopeRandom,
			expected: false,
		},
		{
			name:     "fdRead in LogScopeRandom|LogScopeFilesystem",
			fnd:      fdRead,
			scopes:   logging.LogScopeRandom | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "fdRead in LogScopeAll",
			fnd:      fdRead,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "fdRead not in LogScopeNone",
			fnd:      fdRead,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "pollOneoff in LogScopePoll",
			fnd:      pollOneoff,
			scopes:   logging.LogScopePoll,
			expected: true,
		},
		{
			name:     "pollOneoff not in LogScopeFilesystem",
			fnd:      pollOneoff,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "pollOneoff in LogScopePoll|LogScopeFilesystem",
			fnd:      pollOneoff,
			scopes:   logging.LogScopePoll | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "pollOneoff in LogScopeAll",
			fnd:      pollOneoff,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "pollOneoff not in LogScopeNone",
			fnd:      pollOneoff,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "procExit in LogScopeProc",
			fnd:      procExit,
			scopes:   logging.LogScopeProc,
			expected: true,
		},
		{
			name:     "procExit not in LogScopeFilesystem",
			fnd:      procExit,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "procExit in LogScopeProc|LogScopeFilesystem",
			fnd:      procExit,
			scopes:   logging.LogScopeProc | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "procExit in LogScopeAll",
			fnd:      procExit,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "procExit not in LogScopeNone",
			fnd:      procExit,
			scopes:   logging.LogScopeNone,
			expected: false,
		},
		{
			name:     "randomGet not in LogScopeFilesystem",
			fnd:      randomGet,
			scopes:   logging.LogScopeFilesystem,
			expected: false,
		},
		{
			name:     "randomGet in LogScopeRandom|LogScopeFilesystem",
			fnd:      randomGet,
			scopes:   logging.LogScopeRandom | logging.LogScopeFilesystem,
			expected: true,
		},
		{
			name:     "randomGet in LogScopeAll",
			fnd:      randomGet,
			scopes:   logging.LogScopeAll,
			expected: true,
		},
		{
			name:     "randomGet not in LogScopeNone",
			fnd:      randomGet,
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
