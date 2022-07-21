package sys

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/platform"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func TestContext_FS(t *testing.T) {
	sysCtx := DefaultContext(testfs.FS{})

	require.Equal(t, NewFSContext(testfs.FS{}), sysCtx.FS(testCtx))

	// can override to something else
	fsc := NewFSContext(testfs.FS{"foo": &testfs.File{}})
	require.Equal(t, fsc, sysCtx.FS(context.WithValue(testCtx, FSKey{}, fsc)))
}

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
		nil,         // nanosleep
		testfs.FS{}, // fs
	)
	require.NoError(t, err)

	require.Nil(t, sysCtx.Args())
	require.Zero(t, sysCtx.ArgsSize())
	require.Nil(t, sysCtx.Environ())
	require.Zero(t, sysCtx.EnvironSize())
	require.Equal(t, eofReader{}, sysCtx.Stdin())
	require.Equal(t, io.Discard, sysCtx.Stdout())
	require.Equal(t, io.Discard, sysCtx.Stderr())
	// To compare functions, we can only compare pointers, but the pointer will
	// change. Hence, we have to compare the results instead.
	sec, _ := sysCtx.Walltime(testCtx)
	require.Equal(t, platform.FakeEpochNanos/time.Second.Nanoseconds(), sec)
	require.Equal(t, sys.ClockResolution(1_000), sysCtx.WalltimeResolution())
	require.Zero(t, sysCtx.Nanotime(testCtx)) // See above on functions.
	require.Equal(t, sys.ClockResolution(1), sysCtx.NanotimeResolution())
	require.Equal(t, &ns, sysCtx.nanosleep)
	require.Equal(t, rand.Reader, sysCtx.RandSource())
	require.Equal(t, NewFSContext(testfs.FS{}), sysCtx.FS(testCtx))
}

func TestNewContext_Args(t *testing.T) {
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
				nil, // nanosleep
				nil, // fs
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

func TestNewContext_Environ(t *testing.T) {
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
				nil, // nanosleep
				nil, // fs
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

func TestNewContext_Walltime(t *testing.T) {
	tests := []struct {
		name        string
		time        *sys.Walltime
		resolution  sys.ClockResolution
		expectedErr string
	}{
		{
			name:       "ok",
			time:       platform.NewFakeWalltime(),
			resolution: 3,
		},
		{
			name:        "invalid resolution",
			time:        platform.NewFakeWalltime(),
			resolution:  0,
			expectedErr: "invalid Walltime resolution: 0",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := NewContext(
				0,   // max
				nil, // args
				nil,
				nil,                    // stdin
				nil,                    // stdout
				nil,                    // stderr
				nil,                    // randSource
				tc.time, tc.resolution, // walltime, walltimeResolution
				nil, 0, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.time, sysCtx.walltime)
				require.Equal(t, tc.resolution, sysCtx.WalltimeResolution())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestNewContext_Nanotime(t *testing.T) {
	tests := []struct {
		name        string
		time        *sys.Nanotime
		resolution  sys.ClockResolution
		expectedErr string
	}{
		{
			name:       "ok",
			time:       platform.NewFakeNanotime(),
			resolution: 3,
		},
		{
			name:        "invalid resolution",
			time:        platform.NewFakeNanotime(),
			resolution:  0,
			expectedErr: "invalid Nanotime resolution: 0",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := NewContext(
				0,   // max
				nil, // args
				nil,
				nil,    // stdin
				nil,    // stdout
				nil,    // stderr
				nil,    // randSource
				nil, 0, // nanotime, nanotimeResolution
				tc.time, tc.resolution, // nanotime, nanotimeResolution
				nil, // nanosleep
				nil, // fs
			)
			if tc.expectedErr == "" {
				require.Nil(t, err)
				require.Equal(t, tc.time, sysCtx.nanotime)
				require.Equal(t, tc.resolution, sysCtx.NanotimeResolution())
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func Test_clockResolutionInvalid(t *testing.T) {
	tests := []struct {
		name       string
		resolution sys.ClockResolution
		expected   bool
	}{
		{
			name:       "ok",
			resolution: 1,
		},
		{
			name:       "zero",
			resolution: 0,
			expected:   true,
		},
		{
			name:       "too big",
			resolution: sys.ClockResolution(time.Hour.Nanoseconds() * 2),
			expected:   true,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, clockResolutionInvalid(tc.resolution))
		})
	}
}

func TestNewContext_Nanosleep(t *testing.T) {
	var aNs sys.Nanosleep = func(context.Context, int64) {
	}
	sysCtx, err := NewContext(
		0,   // max
		nil, // args
		nil,
		nil,    // stdin
		nil,    // stdout
		nil,    // stderr
		nil,    // randSource
		nil, 0, // Nanosleep, NanosleepResolution
		nil, 0, // Nanosleep, NanosleepResolution
		&aNs, // nanosleep
		nil,  // fs
	)
	require.Nil(t, err)
	require.Equal(t, &aNs, sysCtx.nanosleep)
}
