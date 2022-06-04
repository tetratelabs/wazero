package sys

import (
	"bytes"
	"io"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func TestDefaultSysContext(t *testing.T) {
	sysCtx, err := NewContext(
		0,      // max
		nil,    // args
		nil,    // environ
		nil,    // stdin
		nil,    // stdout
		nil,    // stderr
		nil,    // randSource
		nil, 0, // walltime, walltimeResolution
		nil, 0, // nanotime, nanotimeResolution
		nil, // openedFiles
	)
	require.NoError(t, err)

	require.Nil(t, sysCtx.Args())
	require.Zero(t, sysCtx.ArgsSize())
	require.Nil(t, sysCtx.Environ())
	require.Zero(t, sysCtx.EnvironSize())
	require.Equal(t, eofReader{}, sysCtx.Stdin())
	require.Equal(t, io.Discard, sysCtx.Stdout())
	require.Equal(t, io.Discard, sysCtx.Stderr())
	require.Equal(t, &wt, sysCtx.walltime)
	require.Equal(t, sys.ClockResolution(1_000), sysCtx.walltimeResolution)
	require.Equal(t, &nt, sysCtx.nanotime)
	require.Equal(t, sys.ClockResolution(1), sysCtx.nanotimeResolution)
	require.Equal(t, sysCtx, DefaultContext())
}

func TestNewSysContext_Args(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		maxSize      uint32
		expectedSize uint32
		expectedErr  string
	}{
		{
			name:         "ok",
			maxSize:      10,
			args:         []string{"a", "bc"},
			expectedSize: 5,
		},
		{
			name:        "exceeds max count",
			maxSize:     1,
			args:        []string{"a", "bc"},
			expectedErr: "args invalid: exceeds maximum count",
		},
		{
			name:        "exceeds max size",
			maxSize:     4,
			args:        []string{"a", "bc"},
			expectedErr: "args invalid: exceeds maximum size",
		},
		{
			name:        "null character",
			maxSize:     10,
			args:        []string{"a", string([]byte{'b', 0})},
			expectedErr: "args invalid: contains NUL character",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := NewContext(
				tc.maxSize, // max
				tc.args,
				nil,                              // environ
				bytes.NewReader(make([]byte, 0)), // stdin
				nil,                              // stdout
				nil,                              // stderr
				nil,                              // randSource
				nil, 0,                           // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.args, sysCtx.Args())
				require.Equal(t, tc.expectedSize, sysCtx.ArgsSize())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestNewSysContext_Environ(t *testing.T) {
	tests := []struct {
		name         string
		environ      []string
		maxSize      uint32
		expectedSize uint32
		expectedErr  string
	}{
		{
			name:         "ok",
			maxSize:      10,
			environ:      []string{"a=b", "c=de"},
			expectedSize: 9,
		},
		{
			name:        "exceeds max count",
			maxSize:     1,
			environ:     []string{"a=b", "c=de"},
			expectedErr: "environ invalid: exceeds maximum count",
		},
		{
			name:        "exceeds max size",
			maxSize:     4,
			environ:     []string{"a=b", "c=de"},
			expectedErr: "environ invalid: exceeds maximum size",
		},
		{
			name:        "null character",
			maxSize:     10,
			environ:     []string{"a=b", string(append([]byte("c=d"), 0))},
			expectedErr: "environ invalid: contains NUL character",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := NewContext(
				tc.maxSize, // max
				nil,        // args
				tc.environ,
				bytes.NewReader(make([]byte, 0)), // stdin
				nil,                              // stdout
				nil,                              // stderr
				nil,                              // randSource
				nil, 0,                           // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // openedFiles
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.environ, sysCtx.Environ())
				require.Equal(t, tc.expectedSize, sysCtx.EnvironSize())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}
