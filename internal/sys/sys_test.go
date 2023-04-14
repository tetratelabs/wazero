package sys

import (
	"bytes"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func TestContext_WalltimeNanos(t *testing.T) {
	sysCtx := DefaultContext(nil)

	require.Equal(t, int64(1640995200000000000), sysCtx.WalltimeNanos())
}

func TestDefaultSysContext(t *testing.T) {
	t.Skip("temporarily skip")

	testFS := sysfs.Adapt(fstest.FS)

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
		nil,    // nanosleep
		nil,    // osyield
		testFS, // rootFS
	)
	require.NoError(t, err)

	require.Nil(t, sysCtx.Args())
	require.Zero(t, sysCtx.ArgsSize())
	require.Nil(t, sysCtx.Environ())
	require.Zero(t, sysCtx.EnvironSize())
	// To compare functions, we can only compare pointers, but the pointer will
	// change. Hence, we have to compare the results instead.
	sec, _ := sysCtx.Walltime()
	require.Equal(t, platform.FakeEpochNanos/time.Second.Nanoseconds(), sec)
	require.Equal(t, sys.ClockResolution(1_000), sysCtx.WalltimeResolution())
	require.Zero(t, sysCtx.Nanotime()) // See above on functions.
	require.Equal(t, sys.ClockResolution(1), sysCtx.NanotimeResolution())
	require.Equal(t, platform.FakeNanosleep, sysCtx.nanosleep)
	require.Equal(t, platform.NewFakeRandSource(), sysCtx.RandSource())

	expectedOpenedFiles := FileTable{}
	expectedOpenedFiles.Insert(noopStdin)
	expectedOpenedFiles.Insert(noopStdout)
	expectedOpenedFiles.Insert(noopStderr)
	expectedOpenedFiles.Insert(&FileEntry{
		IsPreopen: true,
		Name:      "/",
		FS:        testFS,
		File:      &lazyDir{fs: testFS},
	})
	require.Equal(t, expectedOpenedFiles, sysCtx.FS().openedFiles)
}

func TestFileEntry_cachedStat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, fstest.WriteTestFiles(tmpDir))
	dirFS := sysfs.NewDirFS(tmpDir)

	// get the expected inode
	st, errno := platform.Stat(tmpDir)
	require.Zero(t, errno)

	tests := []struct {
		name        string
		fs          sysfs.FS
		expectedIno uint64
	}{
		{name: "sysfs.FS", fs: dirFS, expectedIno: st.Ino},
		{name: "fs.FS", fs: sysfs.Adapt(fstest.FS), expectedIno: 0},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			c := Context{}
			_ = c.NewFSContext(nil, nil, nil, tc.fs)
			fsc := c.fsc
			defer fsc.Close(testCtx)

			f, ok := fsc.LookupFile(FdPreopen)
			require.True(t, ok)
			ino, ft, err := f.CachedStat()
			require.NoError(t, err)
			require.Equal(t, fs.ModeDir, ft)
			if !canReadDirInode() {
				tc.expectedIno = 0
			}
			require.Equal(t, tc.expectedIno, ino)
			require.Equal(t, &cachedStat{Ino: tc.expectedIno, Type: fs.ModeDir}, f.cachedStat)
		})
	}
}

func canReadDirInode() bool {
	if runtime.GOOS != "windows" {
		return true
	} else {
		return strings.HasPrefix(runtime.Version(), "go1.20")
	}
}

func TestNewContext_Args(t *testing.T) {
	tests := []struct {
		name         string
		args         [][]byte
		maxSize      uint32
		expectedSize uint32
		expectedErr  string
	}{
		{
			name:         "ok",
			maxSize:      10,
			args:         [][]byte{[]byte("a"), []byte("bc")},
			expectedSize: 5,
		},
		{
			name:        "exceeds max count",
			maxSize:     1,
			args:        [][]byte{[]byte("a"), []byte("bc")},
			expectedErr: "args invalid: exceeds maximum count",
		},
		{
			name:        "exceeds max size",
			maxSize:     4,
			args:        [][]byte{[]byte("a"), []byte("bc")},
			expectedErr: "args invalid: exceeds maximum size",
		},
		{
			name:        "null character",
			maxSize:     10,
			args:        [][]byte{[]byte("a"), {'b', 0}},
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
				nil, // osyield
				nil, // rootFS
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
		environ      [][]byte
		maxSize      uint32
		expectedSize uint32
		expectedErr  string
	}{
		{
			name:         "ok",
			maxSize:      10,
			environ:      [][]byte{[]byte("a=b"), []byte("c=de")},
			expectedSize: 9,
		},
		{
			name:        "exceeds max count",
			maxSize:     1,
			environ:     [][]byte{[]byte("a=b"), []byte("c=de")},
			expectedErr: "environ invalid: exceeds maximum count",
		},
		{
			name:        "exceeds max size",
			maxSize:     4,
			environ:     [][]byte{[]byte("a=b"), []byte("c=de")},
			expectedErr: "environ invalid: exceeds maximum size",
		},
		{
			name:        "null character",
			maxSize:     10,
			environ:     [][]byte{[]byte("a=b"), append([]byte("c=d"), 0)},
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
				nil, // osyield
				nil, // rootFS
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
		time        sys.Walltime
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
				nil, // osyield
				nil, // rootFS
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
		time        sys.Nanotime
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
				nil, // osyield
				nil, // rootFS
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
	var aNs sys.Nanosleep = func(int64) {}
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
		aNs, // nanosleep
		nil, // osyield
		nil, // rootFS
	)
	require.Nil(t, err)
	require.Equal(t, aNs, sysCtx.nanosleep)
}

func TestNewContext_Osyield(t *testing.T) {
	var oy sys.Osyield = func() {}
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
		nil, // nanosleep
		oy,  // osyield
		nil, // rootFS
	)
	require.Nil(t, err)
	require.Equal(t, oy, sysCtx.osyield)
}
