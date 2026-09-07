package logging

import (
	"context"
	"strings"
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

// Test_logFdstat ensures all of fs_rights_base and fs_rights_inheriting are
// logged. Both are 64-bit fields, and rights are defined up to bit 28.
func Test_logFdstat(t *testing.T) {
	tests := []struct {
		name               string
		filetype           uint8
		fdflags            uint16
		fsRightsBase       uint64
		fsRightsInheriting uint64
		expected           string
	}{
		{
			name:     "zero",
			expected: "{filetype=UNKNOWN,fdflags=,fs_rights_base=,fs_rights_inheriting=}",
		},
		{
			name:               "rights below the 16th bit",
			filetype:           FILETYPE_REGULAR_FILE,
			fdflags:            FD_APPEND,
			fsRightsBase:       uint64(RIGHT_FD_READ | RIGHT_FD_WRITE),
			fsRightsInheriting: uint64(RIGHT_PATH_OPEN),
			expected:           "{filetype=REGULAR_FILE,fdflags=APPEND,fs_rights_base=FD_READ|FD_WRITE,fs_rights_inheriting=PATH_OPEN}",
		},
		{
			name:               "rights at or above the 16th bit",
			filetype:           FILETYPE_DIRECTORY,
			fsRightsBase:       uint64(RIGHT_PATH_RENAME_SOURCE | RIGHT_FD_FILESTAT_GET | RIGHT_POLL_FD_READWRITE),
			fsRightsInheriting: uint64(RIGHT_PATH_UNLINK_FILE | RIGHT_SOCK_SHUTDOWN),
			expected:           "{filetype=DIRECTORY,fdflags=,fs_rights_base=PATH_RENAME_SOURCE|FD_FILESTAT_GET|POLL_FD_READWRITE,fs_rights_inheriting=PATH_UNLINK_FILE|SOCK_SHUTDOWN}",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mem := &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize), Min: 1}
			mod := &wasm.ModuleInstance{MemoryInstance: mem}

			const offset = 8
			buf := mem.Buffer[offset : offset+24]
			le.PutUint16(buf[0:], uint16(tc.filetype))
			le.PutUint16(buf[2:], tc.fdflags)
			le.PutUint64(buf[8:], tc.fsRightsBase)
			le.PutUint64(buf[16:], tc.fsRightsInheriting)

			var out strings.Builder
			logFdstat(0).Log(context.Background(), mod, &out, []uint64{offset})
			require.Equal(t, tc.expected, out.String())
		})
	}
}
