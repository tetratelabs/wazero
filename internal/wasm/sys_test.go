package internalwasm

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemContext_Defaults(t *testing.T) {
	sys, err := NewSystemContext(
		0,   // max
		nil, // args
		nil, // environ
		nil, // stdin
		nil, // stdout
		nil, // stderr
		nil, // openedFiles
	)
	require.NoError(t, err)

	require.Nil(t, sys.Args())
	require.Zero(t, sys.ArgsSize())
	require.Nil(t, sys.Environ())
	require.Zero(t, sys.EnvironSize())
	require.Equal(t, eofReader{}, sys.Stdin())
	require.Equal(t, io.Discard, sys.Stdout())
	require.Equal(t, io.Discard, sys.Stderr())
	require.Empty(t, sys.openedFiles)
}

func TestSystemContext_Args(t *testing.T) {
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
			sys, err := NewSystemContext(
				tc.maxSize, // max
				tc.args,
				nil,                              // environ
				bytes.NewReader(make([]byte, 0)), // stdin
				nil,                              //stdout
				nil,                              // stderr
				nil,                              // openedFiles
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.args, sys.Args())
				require.Equal(t, tc.expectedSize, sys.ArgsSize())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestSystemContext_Environ(t *testing.T) {
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
			sys, err := NewSystemContext(
				tc.maxSize, // max
				nil,        // args
				tc.environ,
				bytes.NewReader(make([]byte, 0)), // stdin
				nil,                              //stdout
				nil,                              // stderr
				nil,                              // openedFiles
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.environ, sys.Environ())
				require.Equal(t, tc.expectedSize, sys.EnvironSize())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}
