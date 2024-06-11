package wasi_snapshot_preview1_test

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"testing"
	gofstest "testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/fsapi"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
	sysapi "github.com/tetratelabs/wazero/sys"
)

func Test_fdAdvise(t *testing.T) {
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceNormal))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceSequential))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceRandom))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceWillNeed))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceDontNeed))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceNoReuse))
	requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdAdviseName, uint64(3), 0, 0, uint64(wasip1.FdAdviceNoReuse+1))
	requireErrnoResult(t, wasip1.ErrnoBadf, mod, wasip1.FdAdviseName, uint64(1111111), 0, 0, uint64(wasip1.FdAdviceNoReuse+1))
}

// Test_fdAllocate only tests it is stubbed for GrainLang per #271
func Test_fdAllocate(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	const fileName = "file.txt"

	// Create the target file.
	realPath := joinPath(tmpDir, fileName)
	require.NoError(t, os.WriteFile(realPath, []byte("0123456789"), 0o600))

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(
		wazero.NewFSConfig().WithDirMount(tmpDir, "/"),
	))
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)
	defer r.Close(testCtx)

	fd, errno := fsc.OpenFile(preopen, fileName, experimentalsys.O_RDWR, 0)
	require.EqualErrno(t, 0, errno)

	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)

	requireSizeEqual := func(exp int64) {
		st, errno := f.File.Stat()
		require.EqualErrno(t, 0, errno)
		require.Equal(t, exp, st.Size)
	}

	t.Run("errors", func(t *testing.T) {
		requireErrnoResult(t, wasip1.ErrnoBadf, mod, wasip1.FdAllocateName, uint64(12345), 0, 0)
		minusOne := int64(-1)
		requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdAllocateName, uint64(fd), uint64(minusOne), uint64(minusOne))
		requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdAllocateName, uint64(fd), 0, uint64(minusOne))
		requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdAllocateName, uint64(fd), uint64(minusOne), 0)
	})

	t.Run("do not change size", func(t *testing.T) {
		for _, tc := range []struct{ offset, length uint64 }{
			{offset: 0, length: 10},
			{offset: 5, length: 5},
			{offset: 4, length: 0},
			{offset: 10, length: 0},
		} {
			// This shouldn't change the size.
			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAllocateName,
				uint64(fd), tc.offset, tc.length)
			requireSizeEqual(10)
		}
	})

	t.Run("increase", func(t *testing.T) {
		// 10 + 10 > the current size -> increase the size.
		requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdAllocateName,
			uint64(fd), 10, 10)
		requireSizeEqual(20)

		// But the original content must be kept.
		buf, err := os.ReadFile(realPath)
		require.NoError(t, err)
		require.Equal(t, "0123456789", string(buf[:10]))
	})

	require.Equal(t, `
==> wasi_snapshot_preview1.fd_allocate(fd=12345,offset=0,len=0)
<== errno=EBADF
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=-1,len=-1)
<== errno=EINVAL
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=0,len=-1)
<== errno=EINVAL
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=-1,len=0)
<== errno=EINVAL
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=0,len=10)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=5,len=5)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=4,len=0)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=10,len=0)
<== errno=ESUCCESS
==> wasi_snapshot_preview1.fd_allocate(fd=4,offset=10,len=10)
<== errno=ESUCCESS
`, "\n"+log.String())
}

func getPreopen(t *testing.T, fsc *sys.FSContext) experimentalsys.FS {
	preopen, ok := fsc.LookupFile(sys.FdPreopen)
	require.True(t, ok)
	return preopen.FS
}

func Test_fdClose(t *testing.T) {
	// fd_close needs to close an open file descriptor. Open two files so that we can tell which is closed.
	path1, path2 := "dir/-", "dir/a-"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	// open both paths without using WASI
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	fdToClose, errno := fsc.OpenFile(preopen, path1, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	fdToKeep, errno := fsc.OpenFile(preopen, path2, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// Close
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdCloseName, uint64(fdToClose))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=4)
<== errno=ESUCCESS
`, "\n"+log.String())

	// Verify fdToClose is closed and removed from the file descriptor table.
	_, ok := fsc.LookupFile(fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.LookupFile(fdToKeep)
	require.True(t, ok)

	log.Reset()
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		requireErrnoResult(t, wasip1.ErrnoBadf, mod, wasip1.FdCloseName, uint64(42)) // 42 is an arbitrary invalid Fd
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=42)
<== errno=EBADF
`, "\n"+log.String())
	})
	log.Reset()
	t.Run("Can close a pre-open", func(t *testing.T) {
		requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdCloseName, uint64(sys.FdPreopen))
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=3)
<== errno=ESUCCESS
`, "\n"+log.String())
	})
}

// Test_fdDatasync only tests that the call succeeds; it's hard to test its effectiveness.
func Test_fdDatasync(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		fd            int32
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_datasync(fd=42)
<== errno=EBADF
`,
		},
		{
			name:          "valid FD",
			fd:            fd,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_datasync(fd=4)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdDatasyncName, uint64(tc.fd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func openPipe(t *testing.T) (*os.File, *os.File) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	return r, w
}

func closePipe(r, w *os.File) {
	r.Close()
	w.Close()
}

func Test_fdFdstatGet(t *testing.T) {
	file, dir := "animals.txt", "sub"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	// replace stdin with a fake TTY file.
	// TODO: Make this easier once we have in-memory sys.File
	stdin, _ := fsc.LookupFile(sys.FdStdin)
	stdinFile, errno := (&sysfs.AdaptFS{FS: &gofstest.MapFS{"stdin": &gofstest.MapFile{
		Mode: fs.ModeDevice | fs.ModeCharDevice | 0o600,
	}}}).OpenFile("stdin", 0, 0)
	require.EqualErrno(t, 0, errno)

	stdin.File = fsapi.Adapt(stdinFile)

	// Make this file writeable, to ensure flags read-back correctly.
	fileFD, errno := fsc.OpenFile(preopen, file, experimentalsys.O_RDWR, 0)
	require.EqualErrno(t, 0, errno)

	dirFD, errno := fsc.OpenFile(preopen, dir, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	tests := []struct {
		name           string
		fd             int32
		resultFdstat   uint32
		expectedMemory []byte
		expectedErrno  wasip1.Errno
		expectedLog    string
	}{
		{
			name: "stdin is a tty",
			fd:   sys.FdStdin,
			expectedMemory: []byte{
				2, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0xdb, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			}, // We shouldn't see RIGHT_FD_SEEK|RIGHT_FD_TELL on a tty file:
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=0)
<== (stat={filetype=CHARACTER_DEVICE,fdflags=,fs_rights_base=FD_DATASYNC|FD_READ|FDSTAT_SET_FLAGS|FD_SYNC|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stdout",
			fd:   sys.FdStdout,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0xff, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=1)
<== (stat={filetype=BLOCK_DEVICE,fdflags=,fs_rights_base=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stderr",
			fd:   sys.FdStderr,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0xff, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=2)
<== (stat={filetype=BLOCK_DEVICE,fdflags=,fs_rights_base=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "root",
			fd:   sys.FdPreopen,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0x19, 0xfe, 0xbf, 0x7, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0xff, 0xff, 0xff, 0xf, 0x0, 0x0, 0x0, 0x0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=3)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=FD_DATASYNC|FDSTAT_SET_FLAGS|FD_SYNC|PATH_CREATE_DIRECTORY|PATH_CREATE_FILE|PATH_LINK_SOURCE|PATH_LINK_TARGET|PATH_OPEN|FD_READDIR|PATH_READLINK,fs_rights_inheriting=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE|PATH_CREATE_DIRECTORY|PATH_CREATE_FILE|PATH_LINK_SOURCE|PATH_LINK_TARGET|PATH_OPEN|FD_READDIR|PATH_READLINK},errno=ESUCCESS)
`,
		},
		{
			name: "file",
			fd:   fileFD,
			expectedMemory: []byte{
				4, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0xff, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=4)
<== (stat={filetype=REGULAR_FILE,fdflags=,fs_rights_base=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "dir",
			fd:   dirFD,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0x19, 0xfe, 0xbf, 0x7, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0xff, 0xff, 0xff, 0xf, 0x0, 0x0, 0x0, 0x0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=FD_DATASYNC|FDSTAT_SET_FLAGS|FD_SYNC|PATH_CREATE_DIRECTORY|PATH_CREATE_FILE|PATH_LINK_SOURCE|PATH_LINK_TARGET|PATH_OPEN|FD_READDIR|PATH_READLINK,fs_rights_inheriting=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE|PATH_CREATE_DIRECTORY|PATH_CREATE_FILE|PATH_LINK_SOURCE|PATH_LINK_TARGET|PATH_OPEN|FD_READDIR|PATH_READLINK},errno=ESUCCESS)
`,
		},
		{
			name:          "bad FD",
			fd:            -1,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=-1)
<== (stat=,errno=EBADF)
`,
		},
		{
			name:          "resultFdstat exceeds the maximum valid address by 1",
			fd:            dirFD,
			resultFdstat:  memorySize - 24 + 1,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5)
<== (stat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdFdstatGetName, uint64(tc.fd), uint64(tc.resultFdstat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdFdstatGet_StdioNonblock(t *testing.T) {
	stdinR, stdinW := openPipe(t)
	defer closePipe(stdinR, stdinW)

	stdoutR, stdoutW := openPipe(t)
	defer closePipe(stdoutR, stdoutW)

	stderrR, stderrW := openPipe(t)
	defer closePipe(stderrR, stderrW)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithStdin(stdinR).
		WithStdout(stdoutW).
		WithStderr(stderrW))
	defer r.Close(testCtx)

	stdin, stdout, stderr := uint64(0), uint64(1), uint64(2)
	requireErrnoResult(t, 0, mod, wasip1.FdFdstatSetFlagsName, stdin, uint64(wasip1.FD_NONBLOCK))
	requireErrnoResult(t, 0, mod, wasip1.FdFdstatSetFlagsName, stdout, uint64(wasip1.FD_NONBLOCK))
	requireErrnoResult(t, 0, mod, wasip1.FdFdstatSetFlagsName, stderr, uint64(wasip1.FD_NONBLOCK))
	log.Reset()

	tests := []struct {
		name           string
		fd             int32
		resultFdstat   uint32
		expectedMemory []byte
		expectedErrno  wasip1.Errno
		expectedLog    string
	}{
		{
			name: "stdin",
			fd:   sys.FdStdin,
			expectedMemory: []byte{
				0, 0, // fs_filetype
				5, 0, 0, 0, 0, 0, // fs_flags
				0xff, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=0)
<== (stat={filetype=UNKNOWN,fdflags=APPEND|NONBLOCK,fs_rights_base=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stdout",
			fd:   sys.FdStdout,
			expectedMemory: []byte{
				0, 0, // fs_filetype
				5, 0, 0, 0, 0, 0, // fs_flags
				0xff, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=1)
<== (stat={filetype=UNKNOWN,fdflags=APPEND|NONBLOCK,fs_rights_base=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stderr",
			fd:   sys.FdStderr,
			expectedMemory: []byte{
				0, 0, // fs_filetype
				5, 0, 0, 0, 0, 0, // fs_flags
				0xff, 0x1, 0xe0, 0x8, 0x0, 0x0, 0x0, 0x0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=2)
<== (stat={filetype=UNKNOWN,fdflags=APPEND|NONBLOCK,fs_rights_base=FD_DATASYNC|FD_READ|FD_SEEK|FDSTAT_SET_FLAGS|FD_SYNC|FD_TELL|FD_WRITE|FD_ADVISE|FD_ALLOCATE,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdFdstatGetName, uint64(tc.fd), uint64(tc.resultFdstat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdFdstatSetFlagsWithTrunc(t *testing.T) {
	tmpDir := t.TempDir()
	fileName := "test"

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().WithDirMount(tmpDir, "/")))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	fd, errno := fsc.OpenFile(preopen, fileName, experimentalsys.O_RDWR|experimentalsys.O_CREAT|experimentalsys.O_EXCL|experimentalsys.O_TRUNC, 0o600)
	require.EqualErrno(t, 0, errno)

	// Write the initial text to the file.
	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)
	n, _ := f.File.Write([]byte("abc"))
	require.Equal(t, n, 3)

	buf, err := os.ReadFile(joinPath(tmpDir, fileName))
	require.NoError(t, err)
	require.Equal(t, "abc", string(buf))

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdFdstatSetFlagsName, uint64(fd), uint64(0))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=4,flags=)
<== errno=ESUCCESS
`, "\n"+log.String())
	log.Reset()

	buf, err = os.ReadFile(joinPath(tmpDir, fileName))
	require.NoError(t, err)
	require.Equal(t, "abc", string(buf))
}

func Test_fdFdstatSetFlags(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	stdinR, stdinW := openPipe(t)
	defer closePipe(stdinR, stdinW)

	stdoutR, stdoutW := openPipe(t)
	defer closePipe(stdoutR, stdoutW)

	stderrR, stderrW := openPipe(t)
	defer closePipe(stderrR, stderrW)

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithStdin(stdinR).
		WithStdout(stdoutW).
		WithStderr(stderrW).
		WithFSConfig(wazero.NewFSConfig().WithDirMount(tmpDir, "/")))
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)
	defer r.Close(testCtx)

	// First, O_CREAT the file with O_APPEND. We use O_EXCL because that
	// triggers an EEXIST error if called a second time with O_CREAT. Our
	// logic should clear O_CREAT preventing this.
	const fileName = "file.txt"
	// Create the target file.
	fd, errno := fsc.OpenFile(preopen, fileName, experimentalsys.O_RDWR|experimentalsys.O_APPEND|experimentalsys.O_CREAT|experimentalsys.O_EXCL, 0o600)
	require.EqualErrno(t, 0, errno)

	// Write the initial text to the file.
	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)
	_, errno = f.File.Write([]byte("0123456789"))
	require.EqualErrno(t, 0, errno)

	writeWazero := func() {
		iovs := uint32(1) // arbitrary offset
		initialMemory := []byte{
			'?',         // `iovs` is after this
			18, 0, 0, 0, // = iovs[0].offset
			4, 0, 0, 0, // = iovs[0].length
			23, 0, 0, 0, // = iovs[1].offset
			2, 0, 0, 0, // = iovs[1].length
			'?',                // iovs[0].offset is after this
			'w', 'a', 'z', 'e', // iovs[0].length bytes
			'?',      // iovs[1].offset is after this
			'r', 'o', // iovs[1].length bytes
			'?',
		}
		iovsCount := uint32(2)       // The count of iovs
		resultNwritten := uint32(26) // arbitrary offset

		ok := mod.Memory().Write(0, initialMemory)
		require.True(t, ok)

		requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2)
<== (nwritten=6,errno=ESUCCESS)
`, "\n"+log.String())
		log.Reset()
	}

	requireFileContent := func(exp string) {
		buf, err := os.ReadFile(joinPath(tmpDir, fileName))
		require.NoError(t, err)
		require.Equal(t, exp, string(buf))
	}

	// with O_APPEND flag, the data is appended to buffer.
	writeWazero()
	requireFileContent("0123456789" + "wazero")

	// Let's remove O_APPEND.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdFdstatSetFlagsName, uint64(fd), uint64(0))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=4,flags=)
<== errno=ESUCCESS
`, "\n"+log.String()) // FIXME? flags==0 prints 'flags='
	log.Reset()

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdSeekName, uint64(fd), uint64(0), uint64(0), uint64(1024))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=0)
<== (newoffset=0,errno=ESUCCESS)
`, "\n"+log.String())
	log.Reset()

	// Without O_APPEND flag, the data is written at the beginning.
	writeWazero()
	requireFileContent("wazero6789" + "wazero")

	// Restore the O_APPEND flag.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdFdstatSetFlagsName, uint64(fd), uint64(wasip1.FD_APPEND))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=4,flags=APPEND)
<== errno=ESUCCESS
`, "\n"+log.String()) // FIXME? flags==1 prints 'flags=APPEND'
	log.Reset()

	// Restoring the O_APPEND flag should not reset fd offset.
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdTellName, uint64(fd), uint64(1024))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_tell(fd=4,result.offset=1024)
<== errno=ESUCCESS
`, "\n"+log.String())
	log.Reset()
	offset, _ := mod.Memory().Read(1024, 4)
	require.Equal(t, offset, []byte{6, 0, 0, 0})

	// with O_APPEND flag, the data is appended to buffer.
	writeWazero()
	requireFileContent("wazero6789" + "wazero" + "wazero")

	t.Run("nonblock", func(t *testing.T) {
		stdin, stdout, stderr := uint64(0), uint64(1), uint64(2)
		requireErrnoResult(t, 0, mod, wasip1.FdFdstatSetFlagsName, stdin, uint64(wasip1.FD_NONBLOCK))
		requireErrnoResult(t, 0, mod, wasip1.FdFdstatSetFlagsName, stdout, uint64(wasip1.FD_NONBLOCK))
		requireErrnoResult(t, 0, mod, wasip1.FdFdstatSetFlagsName, stderr, uint64(wasip1.FD_NONBLOCK))
	})

	t.Run("errors", func(t *testing.T) {
		requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdFdstatSetFlagsName, uint64(fd), uint64(wasip1.FD_DSYNC))
		requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdFdstatSetFlagsName, uint64(fd), uint64(wasip1.FD_RSYNC))
		requireErrnoResult(t, wasip1.ErrnoInval, mod, wasip1.FdFdstatSetFlagsName, uint64(fd), uint64(wasip1.FD_SYNC))
		requireErrnoResult(t, wasip1.ErrnoBadf, mod, wasip1.FdFdstatSetFlagsName, uint64(12345), uint64(wasip1.FD_APPEND))
		requireErrnoResult(t, wasip1.ErrnoIsdir, mod, wasip1.FdFdstatSetFlagsName, uint64(3) /* preopen */, uint64(wasip1.FD_APPEND))
		requireErrnoResult(t, wasip1.ErrnoIsdir, mod, wasip1.FdFdstatSetFlagsName, uint64(3), uint64(wasip1.FD_NONBLOCK))
	})
}

// Test_fdFdstatSetRights only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetRights(t *testing.T) {
	log := requireErrnoNosys(t, wasip1.FdFdstatSetRightsName, 0, 0, 0)
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_fdstat_set_rights(fd=0,fs_rights_base=,fs_rights_inheriting=)
<== errno=ENOSYS
`, log)
}

func Test_fdFilestatGet(t *testing.T) {
	file, dir := "animals.txt", "sub"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	fileFD, errno := fsc.OpenFile(preopen, file, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	dirFD, errno := fsc.OpenFile(preopen, dir, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	tests := []struct {
		name           string
		fd             int32
		resultFilestat uint32
		expectedMemory []byte
		expectedErrno  wasip1.Errno
		expectedLog    string
	}{
		{
			name: "stdin",
			fd:   sys.FdStdin,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				// expect block device because stdin isn't a real file
				1, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0, 0, 0, 0, 0, 0, 0, 0, // atim
				0, 0, 0, 0, 0, 0, 0, 0, // mtim
				0, 0, 0, 0, 0, 0, 0, 0, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=0)
<== (filestat={filetype=BLOCK_DEVICE,size=0,mtim=0},errno=ESUCCESS)
`,
		},
		{
			name: "stdout",
			fd:   sys.FdStdout,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				// expect block device because stdout isn't a real file
				1, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0, 0, 0, 0, 0, 0, 0, 0, // atim
				0, 0, 0, 0, 0, 0, 0, 0, // mtim
				0, 0, 0, 0, 0, 0, 0, 0, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=1)
<== (filestat={filetype=BLOCK_DEVICE,size=0,mtim=0},errno=ESUCCESS)
`,
		},
		{
			name: "stderr",
			fd:   sys.FdStderr,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				// expect block device because stderr isn't a real file
				1, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0, 0, 0, 0, 0, 0, 0, 0, // atim
				0, 0, 0, 0, 0, 0, 0, 0, // mtim
				0, 0, 0, 0, 0, 0, 0, 0, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=2)
<== (filestat={filetype=BLOCK_DEVICE,size=0,mtim=0},errno=ESUCCESS)
`,
		},
		{
			name: "root",
			fd:   sys.FdPreopen,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x7c, 0x78, 0x9d, 0xf2, 0x55, 0x16, // atim
				0x0, 0x0, 0x7c, 0x78, 0x9d, 0xf2, 0x55, 0x16, // mtim
				0x0, 0x0, 0x7c, 0x78, 0x9d, 0xf2, 0x55, 0x16, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=3)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1609459200000000000},errno=ESUCCESS)
`,
		},
		{
			name: "file",
			fd:   fileFD,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				30, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=4)
<== (filestat={filetype=REGULAR_FILE,size=30,mtim=1667482413000000000},errno=ESUCCESS)
`,
		},
		{
			name: "dir",
			fd:   dirFD,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // atim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // mtim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // ctim
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=5)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1640995200000000000},errno=ESUCCESS)
`,
		},
		{
			name:          "bad FD",
			fd:            -1,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=-1)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             dirFD,
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=5)
<== (filestat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdFilestatGetName, uint64(tc.fd), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdFilestatSetSize(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name                     string
		size                     uint64
		content, expectedContent []byte
		expectedLog              string
		expectedErrno            wasip1.Errno
	}{
		{
			name:            "badf",
			content:         []byte("badf"),
			expectedContent: []byte("badf"),
			expectedErrno:   wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_size(fd=5,size=0)
<== errno=EBADF
`,
		},
		{
			name:            "truncate",
			content:         []byte("123456"),
			expectedContent: []byte("12345"),
			size:            5,
			expectedErrno:   wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_size(fd=4,size=5)
<== errno=ESUCCESS
`,
		},
		{
			name:            "truncate to zero",
			content:         []byte("123456"),
			expectedContent: []byte(""),
			size:            0,
			expectedErrno:   wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_size(fd=4,size=0)
<== errno=ESUCCESS
`,
		},
		{
			name:            "truncate to expand",
			content:         []byte("123456"),
			expectedContent: append([]byte("123456"), make([]byte, 100)...),
			size:            106,
			expectedErrno:   wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_size(fd=4,size=106)
<== errno=ESUCCESS
`,
		},
		{
			name:            "large size",
			content:         []byte(""),
			expectedContent: []byte(""),
			size:            math.MaxUint64,
			expectedErrno:   wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_size(fd=4,size=-1)
<== errno=EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			filepath := path.Base(t.Name())
			mod, fd, log, r := requireOpenFile(t, tmpDir, filepath, tc.content, false)
			defer r.Close(testCtx)

			if filepath == "badf" {
				fd++
			}
			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdFilestatSetSizeName, uint64(fd), uint64(tc.size))

			actual, err := os.ReadFile(joinPath(tmpDir, filepath))
			require.NoError(t, err)
			require.Equal(t, tc.expectedContent, actual)

			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdFilestatSetTimes(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		mtime, atime  int64
		flags         uint16
		expectedLog   string
		expectedErrno wasip1.Errno
	}{
		{
			name:          "badf",
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=-1,atim=0,mtim=0,fst_flags=)
<== errno=EBADF
`,
		},
		{
			name:          "a=omit,m=omit",
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=now,m=omit",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			flags:         wasip1.FstflagsAtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=ATIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=omit,m=now",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			flags:         wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=MTIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=now,m=now",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			flags:         wasip1.FstflagsAtimNow | wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=ATIM_NOW|MTIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=now,m=set",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         55555500,
			atime:         1234, // Must be ignored.
			flags:         wasip1.FstflagsAtimNow | wasip1.FstflagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=1234,mtim=55555500,fst_flags=ATIM_NOW|MTIM)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=set,m=now",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         1234, // Must be ignored.
			atime:         55555500,
			flags:         wasip1.FstflagsAtim | wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=55555500,mtim=1234,fst_flags=ATIM|MTIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=set,m=omit",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         1234, // Must be ignored.
			atime:         55555500,
			flags:         wasip1.FstflagsAtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=55555500,mtim=1234,fst_flags=ATIM)
<== errno=ESUCCESS
`,
		},

		{
			name:          "a=omit,m=set",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         55555500,
			atime:         1234, // Must be ignored.
			flags:         wasip1.FstflagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=1234,mtim=55555500,fst_flags=MTIM)
<== errno=ESUCCESS
`,
		},

		{
			name:          "a=set,m=set",
			expectedErrno: wasip1.ErrnoSuccess,
			mtime:         55555500,
			atime:         6666666600,
			flags:         wasip1.FstflagsAtim | wasip1.FstflagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=6666666600,mtim=55555500,fst_flags=ATIM|MTIM)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			filepath := path.Base(t.Name())
			mod, fd, log, r := requireOpenFile(t, tmpDir, filepath, []byte("anything"), false)
			defer r.Close(testCtx)

			sys := mod.(*wasm.ModuleInstance).Sys
			fsc := sys.FS()

			paramFd := fd
			if filepath == "badf" {
				paramFd = -1
			}

			f, ok := fsc.LookupFile(fd)
			require.True(t, ok)

			st, errno := f.File.Stat()
			require.EqualErrno(t, 0, errno)
			prevAtime, prevMtime := st.Atim, st.Mtim

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdFilestatSetTimesName,
				uint64(paramFd), uint64(tc.atime), uint64(tc.mtime),
				uint64(tc.flags),
			)

			if tc.expectedErrno == wasip1.ErrnoSuccess {
				f, ok := fsc.LookupFile(fd)
				require.True(t, ok)

				st, errno = f.File.Stat()
				require.EqualErrno(t, 0, errno)
				if tc.flags&wasip1.FstflagsAtim != 0 {
					require.Equal(t, tc.atime, st.Atim)
				} else if tc.flags&wasip1.FstflagsAtimNow != 0 {
					require.True(t, (sys.WalltimeNanos()-st.Atim) < time.Second.Nanoseconds())
				} else {
					require.Equal(t, prevAtime, st.Atim)
				}
				if tc.flags&wasip1.FstflagsMtim != 0 {
					require.Equal(t, tc.mtime, st.Mtim)
				} else if tc.flags&wasip1.FstflagsMtimNow != 0 {
					require.True(t, (sys.WalltimeNanos()-st.Mtim) < time.Second.Nanoseconds())
				} else {
					require.Equal(t, prevMtime, st.Mtim)
				}
			}
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPread(t *testing.T) {
	tmpDir := t.TempDir()
	mod, fd, log, r := requireOpenFile(t, tmpDir, "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
	}

	iovsCount := uint32(2)    // The count of iovs
	resultNread := uint32(26) // arbitrary offset

	tests := []struct {
		name           string
		offset         int64
		expectedMemory []byte
		expectedLog    string
	}{
		{
			name:   "offset zero",
			offset: 0,
			expectedMemory: append(
				initialMemory,
				'w', 'a', 'z', 'e', // iovs[0].length bytes
				'?',      // iovs[1].offset is after this
				'r', 'o', // iovs[1].length bytes
				'?',        // resultNread is after this
				6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				'?',
			),
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=0)
<== (nread=6,errno=ESUCCESS)
`,
		},
		{
			name:   "offset 2",
			offset: 2,
			expectedMemory: append(
				initialMemory,
				'z', 'e', 'r', 'o', // iovs[0].length bytes
				'?', '?', '?', '?', // resultNread is after this
				4, 0, 0, 0, // sum(iovs[...].length) == length of "zero"
				'?',
			),
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=2)
<== (nread=4,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(tc.offset), uint64(resultNread))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdPread_offset(t *testing.T) {
	tmpDir := t.TempDir()
	mod, fd, log, r := requireOpenFile(t, tmpDir, "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	// Do an initial fdPread.

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
	}
	iovsCount := uint32(2)    // The count of iovs
	resultNread := uint32(26) // arbitrary offset

	expectedMemory := append(
		initialMemory,
		'z', 'e', 'r', 'o', // iovs[0].length bytes
		'?', '?', '?', '?', // resultNread is after this
		4, 0, 0, 0, // sum(iovs[...].length) == length of "zero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))

	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), 2, uint64(resultNread))
	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Verify that the fdPread didn't affect the fdRead offset.

	expectedMemory = append(
		initialMemory,
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',        // resultNread is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
	actual, ok = mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	expectedLog := `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=2)
<== (nread=4,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=2)
<== (nread=6,errno=ESUCCESS)
`
	require.Equal(t, expectedLog, "\n"+log.String())
}

func Test_fdPread_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	contents := []byte("wazero")
	mod, fd, log, r := requireOpenFile(t, tmpDir, "test_path", contents, true)
	defer r.Close(testCtx)

	tests := []struct {
		name                         string
		fd                           int32
		iovs, iovsCount, resultNread uint32
		offset                       int64
		memory                       []byte
		expectedErrno                wasip1.Errno
		expectedLog                  string
	}{
		{
			name:          "invalid FD",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nread validation
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=42,iovs=65532,iovs_len=0,offset=0)
<== (nread=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65536,iovs_len=0,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "out-of-memory reading iovs[0].length",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65532,iovs_len=1,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',          // `iovs` is after this
				0, 0, 0x1, 0, // = iovs[0].offset on the second page
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65528,iovs_len=1,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "length to read exceeds memory by 1",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				0, 0, 0x1, 0, // = iovs[0].length on the second page
				'?',
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65527,iovs_len=1,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "resultNread offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNread: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65527,iovs_len=1,offset=0)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "offset negative",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNread: 10,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
				'?', '?', '?', '?',
			},
			offset:        int64(-1),
			expectedErrno: wasip1.ErrnoIo,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65523,iovs_len=1,offset=-1)
<== (nread=,errno=EIO)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(offset, tc.memory)
			require.True(t, memoryWriteOK)

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdPreadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount), uint64(tc.offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatGet(t *testing.T) {
	fsConfig := wazero.NewFSConfig().WithDirMount(t.TempDir(), "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	resultPrestat := uint32(1) // arbitrary offset
	expectedMemory := []byte{
		'?',     // resultPrestat after this
		0,       // 8-bit tag indicating `prestat_dir`, the only available tag
		0, 0, 0, // 3-byte padding
		// the result path length field after this
		1, 0, 0, 0, // = in little endian encoding
		'?',
	}

	maskMemory(t, mod, len(expectedMemory))

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdPrestatGetName, uint64(sys.FdPreopen), uint64(resultPrestat))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_prestat_get(fd=3)
<== (prestat={pr_name_len=1},errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatGet_Errors(t *testing.T) {
	mod, dirFD, log, r := requireOpenFile(t, t.TempDir(), "tmp", nil, true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
	tests := []struct {
		name          string
		fd            int32
		resultPrestat uint32
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid Fd
			resultPrestat: 0,  // valid offset
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=42)
<== (prestat=,errno=EBADF)
`,
		},
		{
			name:          "not pre-opened FD",
			fd:            dirFD,
			resultPrestat: 0, // valid offset
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=4)
<== (prestat=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory resultPrestat",
			fd:            sys.FdPreopen,
			resultPrestat: memorySize,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=3)
<== (prestat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdPrestatGetName, uint64(tc.fd), uint64(tc.resultPrestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatDirName(t *testing.T) {
	fsConfig := wazero.NewFSConfig().WithDirMount(t.TempDir(), "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(0) // shorter than len("/") to prove truncation is ok
	expectedMemory := []byte{
		'?', '?', '?', '?',
	}

	maskMemory(t, mod, len(expectedMemory))

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdPrestatDirNameName, uint64(sys.FdPreopen), uint64(path), uint64(pathLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatDirName_Errors(t *testing.T) {
	mod, dirFD, log, r := requireOpenFile(t, t.TempDir(), "tmp", nil, true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()
	maskMemory(t, mod, 10)

	validAddress := uint32(0) // Arbitrary valid address as arguments to fd_prestat_dir_name. We chose 0 here.
	pathLen := uint32(len("/"))

	tests := []struct {
		name          string
		fd            int32
		path          uint32
		pathLen       uint32
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "out-of-memory path",
			fd:            sys.FdPreopen,
			path:          memorySize,
			pathLen:       pathLen,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=EFAULT)
`,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            sys.FdPreopen,
			path:          memorySize - pathLen + 1,
			pathLen:       pathLen,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=EFAULT)
`,
		},
		{
			name:          "pathLen exceeds the length of the dir name",
			fd:            sys.FdPreopen,
			path:          validAddress,
			pathLen:       pathLen + 1,
			expectedErrno: wasip1.ErrnoNametoolong,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3)
<== (path=,errno=ENAMETOOLONG)
`,
		},
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=42)
<== (path=,errno=EBADF)
`,
		},
		{
			name:          "not pre-opened FD",
			fd:            dirFD,
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=4)
<== (path=,errno=EBADF)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdPrestatDirNameName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPwrite(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
	}

	iovsCount := uint32(2) // The count of iovs
	resultNwritten := len(initialMemory) + 1

	tests := []struct {
		name             string
		offset           int64
		expectedMemory   []byte
		expectedContents string
		expectedLog      string
	}{
		{
			name:   "offset zero",
			offset: 0,
			expectedMemory: append(
				initialMemory,
				'?',        // resultNwritten is after this
				6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				'?',
			),
			expectedContents: "wazero",
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=1,iovs_len=2,offset=0)
<== (nwritten=6,errno=ESUCCESS)
`,
		},
		{
			name:   "offset 2",
			offset: 2,
			expectedMemory: append(
				initialMemory,
				'?',        // resultNwritten is after this
				6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
				'?',
			),
			expectedContents: "wawazero", // "wa" from the first test!
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=1,iovs_len=2,offset=2)
<== (nwritten=6,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			ok := mod.Memory().Write(0, initialMemory)
			require.True(t, ok)

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdPwriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(tc.offset), uint64(resultNwritten))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)

			// Ensure the contents were really written
			b, err := os.ReadFile(joinPath(tmpDir, pathName))
			require.NoError(t, err)
			require.Equal(t, tc.expectedContents, string(b))
		})
	}
}

func Test_fdPwrite_offset(t *testing.T) {
	tmpDir := t.TempDir()
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	// Do an initial fdPwrite.

	iovs := uint32(1) // arbitrary offset
	pwriteMemory := []byte{
		'?',         // `iovs` is after this
		10, 0, 0, 0, // = iovs[0].offset
		3, 0, 0, 0, // = iovs[0].length
		'?',
		'e', 'r', 'o', // iovs[0].length bytes
		'?', // resultNwritten is after this
	}
	iovsCount := uint32(1) // The count of iovs
	resultNwritten := len(pwriteMemory) + 4

	expectedMemory := append(
		pwriteMemory,
		'?', '?', '?', '?', // resultNwritten is after this
		3, 0, 0, 0, // sum(iovs[...].length) == length of "ero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))

	ok := mod.Memory().Write(0, pwriteMemory)
	require.True(t, ok)

	// Write the last half first, to offset 3
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdPwriteName, uint64(fd), uint64(iovs), uint64(iovsCount), 3, uint64(resultNwritten))
	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Verify that the fdPwrite didn't affect the fdWrite offset.
	writeMemory := []byte{
		'?',         // `iovs` is after this
		10, 0, 0, 0, // = iovs[0].offset
		3, 0, 0, 0, // = iovs[0].length
		'?',
		'w', 'a', 'z', // iovs[0].length bytes
		'?', // resultNwritten is after this
	}
	expectedMemory = append(
		writeMemory,
		'?', '?', '?', '?', // resultNwritten is after this
		3, 0, 0, 0, // sum(iovs[...].length) == length of "waz"
		'?',
	)

	ok = mod.Memory().Write(0, writeMemory)
	require.True(t, ok)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	actual, ok = mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	expectedLog := `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=1,iovs_len=1,offset=3)
<== (nwritten=3,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=1)
<== (nwritten=3,errno=ESUCCESS)
`
	require.Equal(t, expectedLog, "\n"+log.String())

	// Ensure the contents were really written
	b, err := os.ReadFile(joinPath(tmpDir, pathName))
	require.NoError(t, err)
	require.Equal(t, "wazero", string(b))
}

func Test_fdPwrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	tests := []struct {
		name                            string
		fd                              int32
		iovs, iovsCount, resultNwritten uint32
		offset                          int64
		memory                          []byte
		expectedErrno                   wasip1.Errno
		expectedLog                     string
	}{
		{
			name:          "invalid FD",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nwritten validation
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=42,iovs=65532,iovs_len=0,offset=0)
<== (nwritten=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory writing iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=65536,iovs_len=0,offset=0)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name: "out-of-memory writing iovs[0].length",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=65532,iovs_len=1,offset=0)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',          // `iovs` is after this
				0, 0, 0x1, 0, // = iovs[0].offset on the second page
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=65528,iovs_len=1,offset=0)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name: "length to write exceeds memory by 1",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				0, 0, 0x1, 0, // = iovs[0].length on the second page
				'?',
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=65527,iovs_len=1,offset=0)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name: "resultNwritten offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNwritten: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=65527,iovs_len=1,offset=0)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name: "offset negative",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNwritten: 10,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
				'?', '?', '?', '?',
			},
			offset:        int64(-1),
			expectedErrno: wasip1.ErrnoIo,
			expectedLog: `
==> wasi_snapshot_preview1.fd_pwrite(fd=4,iovs=65523,iovs_len=1,offset=-1)
<== (nwritten=,errno=EIO)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(offset, tc.memory)
			require.True(t, memoryWriteOK)

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdPwriteName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount), uint64(tc.offset), uint64(tc.resultNwritten+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdRead(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		26, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		31, 0, 0, 0, // = iovs[1].offset
		0, 0, 0, 0, // = iovs[1].length == 0 !!
		31, 0, 0, 0, // = iovs[2].offset
		2, 0, 0, 0, // = iovs[2].length
		'?',
	}
	iovsCount := uint32(3)    // The count of iovs
	resultNread := uint32(34) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[2].offset is after this
		'r', 'o', // iovs[2].length bytes
		'?',        // resultNread is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))

	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=3)
<== (nread=6,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdRead_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	tests := []struct {
		name                         string
		fd                           int32
		iovs, iovsCount, resultNread uint32
		memory                       []byte
		expectedErrno                wasip1.Errno
		expectedLog                  string
	}{
		{
			name:          "invalid FD",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nread validation
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=42,iovs=65532,iovs_len=65532)
<== (nread=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65536,iovs_len=65535)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "out-of-memory reading iovs[0].length",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65532,iovs_len=65532)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "iovs[0].offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',          // `iovs` is after this
				0, 0, 0x1, 0, // = iovs[0].offset on the second page
				1, 0, 0, 0, // = iovs[0].length
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65528,iovs_len=65528)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "length to read exceeds memory by 1",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				0, 0, 0x1, 0, // = iovs[0].length on the second page
				'?',
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527)
<== (nread=,errno=EFAULT)
`,
		},
		{
			name: "resultNread offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultNread: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527)
<== (nread=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(offset, tc.memory)
			require.True(t, memoryWriteOK)

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdReadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

var (
	direntDot = []byte{
		1, 0, 0, 0, 0, 0, 0, 0, // d_next = 1
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		1, 0, 0, 0, // d_namlen = 1 character
		3, 0, 0, 0, // d_type =  directory
		'.', // name
	}
	direntDotDot = []byte{
		2, 0, 0, 0, 0, 0, 0, 0, // d_next = 2
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		2, 0, 0, 0, // d_namlen = 2 characters
		3, 0, 0, 0, // d_type =  directory
		'.', '.', // name
	}
	dirent1 = []byte{
		3, 0, 0, 0, 0, 0, 0, 0, // d_next = 3
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		1, 0, 0, 0, // d_namlen = 1 character
		4, 0, 0, 0, // d_type = regular_file
		'-', // name
	}
	dirent2 = []byte{
		4, 0, 0, 0, 0, 0, 0, 0, // d_next = 4
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		2, 0, 0, 0, // d_namlen = 1 character
		3, 0, 0, 0, // d_type =  directory
		'a', '-', // name
	}
	dirent3 = []byte{
		5, 0, 0, 0, 0, 0, 0, 0, // d_next = 5
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		3, 0, 0, 0, // d_namlen = 3 characters
		4, 0, 0, 0, // d_type = regular_file
		'a', 'b', '-', // name
	}

	// TODO: this entry is intended to test reading of a symbolic link entry,
	// tho it requires modifying fstest.FS to contain this file.
	// dirent4 = []byte{
	// 	6, 0, 0, 0, 0, 0, 0, 0, // d_next = 6
	// 	0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
	// 	2, 0, 0, 0, // d_namlen = 2 characters
	// 	7, 0, 0, 0, // d_type = symbolic_link
	// 	'l', 'n', // name
	// }

	dirents = bytes.Join([][]byte{
		direntDot,
		direntDotDot,
		dirent1,
		dirent2,
		dirent3,
		// dirent4,
	}, nil)
)

func Test_fdReaddir(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)
	fd := sys.FdPreopen + 1

	tests := []struct {
		name            string
		initialDir      string
		dir             func()
		bufLen          uint32
		cookie          int64
		expectedMem     []byte
		expectedMemSize int
		expectedBufused uint32
	}{
		{
			name:            "empty dir",
			initialDir:      "emptydir",
			bufLen:          wasip1.DirentSize + 1, // size of one entry
			cookie:          0,
			expectedBufused: wasip1.DirentSize + 1, // one dot entry
			expectedMem:     direntDot,
		},
		{
			name:            "full read",
			initialDir:      "dir",
			bufLen:          4096,
			cookie:          0,
			expectedBufused: 129, // length of all entries
			expectedMem:     dirents,
		},
		{
			name:            "can't read name",
			initialDir:      "dir",
			bufLen:          wasip1.DirentSize, // length is long enough for first, but not the name.
			cookie:          0,
			expectedBufused: wasip1.DirentSize,             // == bufLen which is the size of the dirent
			expectedMem:     direntDot[:wasip1.DirentSize], // header without name
		},
		{
			name:            "read exactly first",
			initialDir:      "dir",
			bufLen:          25, // length is long enough for first + the name, but not more.
			cookie:          0,
			expectedBufused: 25, // length to read exactly first.
			expectedMem:     direntDot,
		},
		{
			name:       "read exactly second",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 1)
			},
			bufLen:          27, // length is long enough for exactly second.
			cookie:          1,  // d_next of first
			expectedBufused: 27, // length to read exactly second.
			expectedMem:     direntDotDot,
		},
		{
			name:       "read second and a little more",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 1)
			},
			bufLen:          30, // length is longer than the second entry, but not long enough for a header.
			cookie:          1,  // d_next of first
			expectedBufused: 30, // length to read some more, but not enough for a header, so buf was exhausted.
			expectedMem:     direntDotDot,
			expectedMemSize: len(direntDotDot), // we do not want to compare the full buffer since we don't know what the leftover 4 bytes will contain.
		},
		{
			name:       "read second and header of third",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 1)
			},
			bufLen:          50, // length is longer than the second entry + enough for the header of third.
			cookie:          1,  // d_next of first
			expectedBufused: 50, // length to read exactly second and the header of third.
			expectedMem:     append(direntDotDot, dirent1[0:24]...),
		},
		{
			name:       "read second and third",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 1)
			},
			bufLen:          53, // length is long enough for second and third.
			cookie:          1,  // d_next of first
			expectedBufused: 53, // length to read exactly one second and third.
			expectedMem:     append(direntDotDot, dirent1...),
		},
		{
			name:       "read exactly third",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 2)
			},
			bufLen:          27, // length is long enough for exactly third.
			cookie:          2,  // d_next of second.
			expectedBufused: 27, // length to read exactly third.
			expectedMem:     dirent1,
		},
		{
			name:       "read third and beyond",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 2)
			},
			bufLen:          300, // length is long enough for third and more
			cookie:          2,   // d_next of second.
			expectedBufused: 78,  // length to read the rest
			expectedMem:     append(dirent1, dirent2...),
		},
		{
			name:       "read exhausted directory",
			initialDir: "dir",
			dir: func() {
				f, _ := fsc.LookupFile(fd)
				rdd, _ := f.DirentCache()
				_, _ = rdd.Read(0, 5)
			},
			bufLen:          300, // length is long enough for third and more
			cookie:          5,   // d_next after entries.
			expectedBufused: 0,   // nothing read
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			fd, errno := fsc.OpenFile(preopen, tc.initialDir, experimentalsys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			defer fsc.CloseFile(fd) // nolint

			if tc.dir != nil {
				tc.dir()
			}

			maskMemory(t, mod, int(tc.bufLen))

			resultBufused := uint32(0) // where to write the amount used out of bufLen
			buf := uint32(8)           // where to start the dirents
			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdReaddirName,
				uint64(fd), uint64(buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(resultBufused))

			// read back the bufused and compare memory against it
			bufused, ok := mod.Memory().ReadUint32Le(resultBufused)
			require.True(t, ok)
			require.Equal(t, tc.expectedBufused, bufused)

			mem, ok := mod.Memory().Read(buf, bufused)
			require.True(t, ok)

			if tc.expectedMem != nil {
				if tc.expectedMemSize == 0 {
					tc.expectedMemSize = len(tc.expectedMem)
				}
				require.Equal(t, tc.expectedMem, mem[:tc.expectedMemSize])
			}
		})
	}
}

// This is similar to https://github.com/WebAssembly/wasi-testsuite/blob/ac32f57400cdcdd0425d3085c24fc7fc40011d1c/tests/rust/src/bin/fd_readdir.rs#L120
func Test_fdReaddir_Rewind(t *testing.T) {
	tmpDir := t.TempDir()

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(os.DirFS(tmpDir)))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()

	preopen := getPreopen(t, fsc)
	fd, errno := fsc.OpenFile(preopen, ".", experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	mem := mod.Memory()
	const resultBufused, buf = 0, 8
	fdReaddir := func(cookie uint64) uint32 {
		requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdReaddirName,
			uint64(fd), buf, 256, cookie, uint64(resultBufused))
		bufused, ok := mem.ReadUint32Le(resultBufused)
		require.True(t, ok)
		return bufused
	}

	// Read the empty directory, which should only have the dot entries.
	bufused := fdReaddir(0)
	dotDirentsLen := (wasip1.DirentSize + 1) + (wasip1.DirentSize + 2)
	require.Equal(t, dotDirentsLen, bufused)

	// Write a new file to the directory
	fileName := "file"
	require.NoError(t, os.WriteFile(path.Join(tmpDir, fileName), nil, 0o0666))
	fileDirentLen := wasip1.DirentSize + uint32(len(fileName))

	// Read it again, which should see the new file.
	bufused = fdReaddir(0)
	require.Equal(t, dotDirentsLen+fileDirentLen, bufused)

	// Read it again, using the file position.
	bufused = fdReaddir(2)
	require.Equal(t, fileDirentLen, bufused)

	require.Equal(t, `
==> wasi_snapshot_preview1.fd_readdir(fd=4,buf=8,buf_len=256,cookie=0)
<== (bufused=51,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_readdir(fd=4,buf=8,buf_len=256,cookie=0)
<== (bufused=79,errno=ESUCCESS)
==> wasi_snapshot_preview1.fd_readdir(fd=4,buf=8,buf_len=256,cookie=2)
<== (bufused=28,errno=ESUCCESS)
`, "\n"+log.String())
}

func Test_fdReaddir_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memLen := mod.Memory().Size()

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	fileFD, errno := fsc.OpenFile(preopen, "animals.txt", experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// Directories are stateful, so we open them during the test.
	dirFD := fileFD + 1

	tests := []struct {
		name                       string
		fd                         int32
		buf, bufLen, resultBufused uint32
		cookie                     int64
		expectedErrno              wasip1.Errno
		expectedLog                string
	}{
		{
			name:          "out-of-memory reading buf",
			fd:            dirFD,
			buf:           memLen,
			bufLen:        1000,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=65536,buf_len=1000,cookie=0)
<== (bufused=,errno=EFAULT)
`,
		},
		{
			name: "invalid FD",
			fd:   42,                           // arbitrary invalid fd
			buf:  0, bufLen: wasip1.DirentSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=42,buf=0,buf_len=24,cookie=0)
<== (bufused=,errno=EBADF)
`,
		},
		{
			name: "not a dir",
			fd:   fileFD,
			buf:  0, bufLen: wasip1.DirentSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=24,cookie=0)
<== (bufused=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory reading bufLen",
			fd:            dirFD,
			buf:           memLen - 1,
			bufLen:        1000,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=65535,buf_len=1000,cookie=0)
<== (bufused=,errno=EFAULT)
`,
		},
		{
			name: "bufLen must be enough to write a struct",
			fd:   dirFD,
			buf:  0, bufLen: 1,
			resultBufused: 1000,
			expectedErrno: wasip1.ErrnoInval, // Arbitrary error choice.
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1,cookie=0)
<== (bufused=,errno=EINVAL)
`,
		},
		{
			name: "cookie invalid when no prior state",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        1,
			resultBufused: 2000,
			expectedErrno: wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1000,cookie=1)
<== (bufused=,errno=ENOENT)
`,
		},
		{
			// cookie should be treated opaquely. When negative, it is a
			// position not yet read,
			name: "negative cookie invalid",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        -1,
			resultBufused: 2000,
			expectedErrno: wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1000,cookie=-1)
<== (bufused=,errno=ENOENT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Reset the directory so that tests don't taint each other.
			if tc.fd == dirFD {
				dirFD, errno = fsc.OpenFile(preopen, "dir", experimentalsys.O_RDONLY, 0)
				require.EqualErrno(t, 0, errno)
				defer fsc.CloseFile(dirFD) // nolint
			}

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdReaddirName,
				uint64(tc.fd), uint64(tc.buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(tc.resultBufused))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdRenumber(t *testing.T) {
	const fileFD, dirFD = 4, 5

	tests := []struct {
		name          string
		from, to      int32
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "from=preopen",
			from:          sys.FdPreopen,
			to:            dirFD,
			expectedErrno: wasip1.ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=3,to=5)
<== errno=ENOTSUP
`,
		},
		{
			name:          "from=badf",
			from:          -1,
			to:            sys.FdPreopen,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=-1,to=3)
<== errno=EBADF
`,
		},
		{
			name:          "to=badf",
			from:          sys.FdPreopen,
			to:            -1,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=3,to=-1)
<== errno=EBADF
`,
		},
		{
			name:          "to=preopen",
			from:          dirFD,
			to:            sys.FdPreopen,
			expectedErrno: wasip1.ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=5,to=3)
<== errno=ENOTSUP
`,
		},
		{
			name:          "file to dir",
			from:          fileFD,
			to:            dirFD,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=4,to=5)
<== errno=ESUCCESS
`,
		},
		{
			name:          "dir to file",
			from:          dirFD,
			to:            fileFD,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=5,to=4)
<== errno=ESUCCESS
`,
		},
		{
			name:          "dir to any",
			from:          dirFD,
			to:            12345,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=5,to=12345)
<== errno=ESUCCESS
`,
		},
		{
			name:          "file to any",
			from:          fileFD,
			to:            54,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=4,to=54)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
			defer r.Close(testCtx)

			fsc := mod.(*wasm.ModuleInstance).Sys.FS()
			preopen := getPreopen(t, fsc)

			// Sanity check of the file descriptor assignment.
			fileFDAssigned, errno := fsc.OpenFile(preopen, "animals.txt", experimentalsys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, int32(fileFD), fileFDAssigned)

			dirFDAssigned, errno := fsc.OpenFile(preopen, "dir", experimentalsys.O_RDONLY, 0)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, int32(dirFD), dirFDAssigned)

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdRenumberName, uint64(tc.from), uint64(tc.to))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdSeek(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	resultNewoffset := uint32(1) // arbitrary offset in api.Memory for the new offset value

	tests := []struct {
		name           string
		offset         int64
		whence         int
		expectedOffset int64
		expectedMemory []byte
		expectedLog    string
	}{
		{
			name:           "SeekStart",
			offset:         4, // arbitrary offset
			whence:         io.SeekStart,
			expectedOffset: 4, // = offset
			expectedMemory: []byte{
				'?',                    // resultNewoffset is after this
				4, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=4,whence=0)
<== (newoffset=4,errno=ESUCCESS)
`,
		},
		{
			name:           "SeekCurrent",
			offset:         1, // arbitrary offset
			whence:         io.SeekCurrent,
			expectedOffset: 2, // = 1 (the initial offset of the test file) + 1 (offset)
			expectedMemory: []byte{
				'?',                    // resultNewoffset is after this
				2, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=1,whence=1)
<== (newoffset=2,errno=ESUCCESS)
`,
		},
		{
			name:           "SeekEnd",
			offset:         -1, // arbitrary offset, note that offset can be negative
			whence:         io.SeekEnd,
			expectedOffset: 5, // = 6 (the size of the test file with content "wazero") + -1 (offset)
			expectedMemory: []byte{
				'?',                    // resultNewoffset is after this
				5, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=-1,whence=2)
<== (newoffset=5,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			// Since we initialized this file, we know it is a seeker (because it is a MapFile)
			fsc := mod.(*wasm.ModuleInstance).Sys.FS()
			f, ok := fsc.LookupFile(fd)
			require.True(t, ok)

			// set the initial offset of the file to 1
			offset, errno := f.File.Seek(1, io.SeekStart)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, int64(1), offset)

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdSeekName, uint64(fd), uint64(tc.offset), uint64(tc.whence), uint64(resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)

			offset, errno = f.File.Seek(0, io.SeekCurrent)
			require.EqualErrno(t, 0, errno)
			require.Equal(t, tc.expectedOffset, offset) // test that the offset of file is actually updated.
		})
	}
}

func Test_fdSeek_Errors(t *testing.T) {
	mod, fileFD, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), false)
	defer r.Close(testCtx)

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)
	require.Zero(t, preopen.Mkdir("dir", 0o0777))
	dirFD := requireOpenFD(t, mod, "dir")

	memorySize := mod.Memory().Size()

	tests := []struct {
		name                    string
		fd                      int32
		offset                  uint64
		whence, resultNewoffset uint32
		expectedErrno           wasip1.Errno
		expectedLog             string
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=42,offset=0,whence=0)
<== (newoffset=,errno=EBADF)
`,
		},
		{
			name:          "invalid whence",
			fd:            fileFD,
			whence:        3, // invalid whence, the largest whence io.SeekEnd(2) + 1
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=3)
<== (newoffset=,errno=EINVAL)
`,
		},
		{
			name:          "dir not file",
			fd:            dirFD,
			expectedErrno: wasip1.ErrnoIsdir,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=5,offset=0,whence=0)
<== (newoffset=,errno=EISDIR)
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fileFD,
			resultNewoffset: memorySize,
			expectedErrno:   wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=0)
<== (newoffset=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdSeekName, uint64(tc.fd), tc.offset, uint64(tc.whence), uint64(tc.resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdSync only tests that the call succeeds; it's hard to test its effectiveness.
func Test_fdSync(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		fd            int32
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_sync(fd=42)
<== errno=EBADF
`,
		},
		{
			name:          "valid FD",
			fd:            fd,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_sync(fd=4)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdSyncName, uint64(tc.fd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdTell(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)
	defer log.Reset()

	resultNewoffset := uint32(1) // arbitrary offset in api.Memory for the new offset value

	expectedOffset := int64(1) // = offset
	expectedMemory := []byte{
		'?',                    // resultNewoffset is after this
		1, 0, 0, 0, 0, 0, 0, 0, // = expectedOffset
		'?',
	}
	expectedLog := `
==> wasi_snapshot_preview1.fd_tell(fd=4,result.offset=1)
<== errno=ESUCCESS
`

	maskMemory(t, mod, len(expectedMemory))

	// Since we initialized this file, we know it is a seeker (because it is a MapFile)
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)

	// set the initial offset of the file to 1
	offset, errno := f.File.Seek(1, io.SeekStart)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, int64(1), offset)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdTellName, uint64(fd), uint64(resultNewoffset))
	require.Equal(t, expectedLog, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	offset, errno = f.File.Seek(0, io.SeekCurrent)
	require.EqualErrno(t, 0, errno)
	require.Equal(t, expectedOffset, offset) // test that the offset of file is actually updated.
}

func Test_fdTell_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()

	tests := []struct {
		name            string
		fd              int32
		resultNewoffset uint32
		expectedErrno   wasip1.Errno
		expectedLog     string
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_tell(fd=42,result.offset=0)
<== errno=EBADF
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fd,
			resultNewoffset: memorySize,
			expectedErrno:   wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_tell(fd=4,result.offset=65536)
<== errno=EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdTellName, uint64(tc.fd), uint64(tc.resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdWrite(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	iovs := uint32(1) // arbitrary offset
	initialMemory := []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',                // iovs[0].offset is after this
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',
	}
	iovsCount := uint32(2)       // The count of iovs
	resultNwritten := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))
	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2)
<== (nwritten=6,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Since we initialized this file, we know we can read it by path
	buf, err := os.ReadFile(joinPath(tmpDir, pathName))
	require.NoError(t, err)

	require.Equal(t, []byte("wazero"), buf) // verify the file was actually written
}

func Test_fdWrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{1, 2, 3, 4}, false)
	defer r.Close(testCtx)

	// Setup valid test memory
	iovsCount := uint32(1)
	memSize := mod.Memory().Size()

	tests := []struct {
		name                 string
		fd                   int32
		iovs, resultNwritten uint32
		expectedErrno        wasip1.Errno
		expectedLog          string
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=42,iovs=0,iovs_len=1)
<== (nwritten=,errno=EBADF)
`,
		},
		{
			name:          "not writable FD",
			fd:            sys.FdStdin,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog:   "\n", // stdin is not sampled
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          memSize - 2,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65534,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].length",
			fd:            fd,
			iovs:          memSize - 4, // iovs[0].offset was 4 bytes and iovs[0].length next, but not enough mod.Memory()!
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65532,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "iovs[0].offset is outside memory",
			fd:            fd,
			iovs:          memSize - 5, // iovs[0].offset (where to read "hi") is outside memory.
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65531,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "length to read exceeds memory by 1",
			fd:            fd,
			iovs:          memSize - 7, // iovs[0].offset (where to read "hi") is in memory, but truncated.
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65529,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:           "resultNwritten offset is outside memory",
			fd:             fd,
			resultNwritten: memSize, // read was ok, but there wasn't enough memory to write the result.
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.iovs, append(
				u64.LeBytes(uint64(tc.iovs+8)), // = iovs[0].offset (where the data "hi" begins)
				// = iovs[0].length (how many bytes are in "hi")
				2, 0, 0, 0,
				'h', 'i', // iovs[0].length bytes
			))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.FdWriteName, uint64(tc.fd), uint64(tc.iovs), uint64(iovsCount),
				uint64(tc.resultNwritten))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathCreateDirectory(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	realPath := joinPath(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	fd := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathCreateDirectoryName, uint64(fd), uint64(name), uint64(nameLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=wazero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the directory was created
	stat, err := os.Stat(realPath)
	require.NoError(t, err)
	require.True(t, stat.IsDir())
	require.Equal(t, pathName, stat.Name())
}

func Test_pathCreateDirectory_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	file := "file"
	err := os.WriteFile(joinPath(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(joinPath(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName string
		fd             int32
		path, pathLen  uint32
		expectedErrno  wasip1.Errno
		expectedLog    string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "Fd not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=4,path=file)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       1,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "file exists",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoExist,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=file)
<== errno=EEXIST
`,
		},
		{
			name:          "dir exists",
			fd:            sys.FdPreopen,
			pathName:      dir,
			path:          0,
			pathLen:       uint32(len(dir)),
			expectedErrno: wasip1.ErrnoExist,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=3,path=dir)
<== errno=EEXIST
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathCreateDirectoryName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathFilestatGet(t *testing.T) {
	file, dir, fileInDir := "animals.txt", "sub", "sub/test.txt"

	initialMemoryFile := append([]byte{'?'}, file...)
	initialMemoryDir := append([]byte{'?'}, dir...)
	initialMemoryFileInDir := append([]byte{'?'}, fileInDir...)
	initialMemoryNotExists := []byte{'?', '?'}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	fileFD := requireOpenFD(t, mod, file)

	tests := []struct {
		name                    string
		fd                      int32
		pathLen, resultFilestat uint32
		flags                   uint16
		memory, expectedMemory  []byte
		expectedErrno           wasip1.Errno
		expectedLog             string
	}{
		{
			name:           "file under root",
			fd:             sys.FdPreopen,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: uint32(len(file)) + 1,
			expectedMemory: append(
				initialMemoryFile,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				30, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=animals.txt)
<== (filestat={filetype=REGULAR_FILE,size=30,mtim=1667482413000000000},errno=ESUCCESS)
`,
		},
		{
			name:           "file under dir",
			fd:             sys.FdPreopen, // root
			memory:         initialMemoryFileInDir,
			pathLen:        uint32(len(fileInDir)),
			resultFilestat: uint32(len(fileInDir)) + 1,
			expectedMemory: append(
				initialMemoryFileInDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				14, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // atim
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // mtim
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=sub/test.txt)
<== (filestat={filetype=REGULAR_FILE,size=14,mtim=1672531200000000000},errno=ESUCCESS)
`,
		},
		{
			name:           "dir under root",
			fd:             sys.FdPreopen,
			memory:         initialMemoryDir,
			pathLen:        uint32(len(dir)),
			resultFilestat: uint32(len(dir)) + 1,
			expectedMemory: append(
				initialMemoryDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // atim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // mtim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=sub)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1640995200000000000},errno=ESUCCESS)
`,
		},
		{
			name:          "unopened FD",
			fd:            -1,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=-1,flags=,path=)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "Fd not a directory",
			fd:             fileFD,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: 2,
			expectedErrno:  wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=4,flags=,path=animals.txt)
<== (filestat=,errno=ENOTDIR)
`,
		},
		{
			name:           "path under root doesn't exist",
			fd:             sys.FdPreopen,
			memory:         initialMemoryNotExists,
			pathLen:        1,
			resultFilestat: 2,
			expectedErrno:  wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=?)
<== (filestat=,errno=ENOENT)
`,
		},
		{
			name:          "path is out of memory",
			fd:            sys.FdPreopen,
			memory:        initialMemoryFile,
			pathLen:       memorySize,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=OOM(1,65536))
<== (filestat=,errno=EFAULT)
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             sys.FdPreopen,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=animals.txt)
<== (filestat=,errno=EFAULT)
`,
		},
		{
			name:           "file under root (follow symlinks)",
			fd:             sys.FdPreopen,
			flags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: uint32(len(file)) + 1,
			expectedMemory: append(
				initialMemoryFile,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				30, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=SYMLINK_FOLLOW,path=animals.txt)
<== (filestat={filetype=REGULAR_FILE,size=30,mtim=1667482413000000000},errno=ESUCCESS)
`,
		},
		{
			name:           "file under dir (follow symlinks)",
			fd:             sys.FdPreopen, // root
			flags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:         initialMemoryFileInDir,
			pathLen:        uint32(len(fileInDir)),
			resultFilestat: uint32(len(fileInDir)) + 1,
			expectedMemory: append(
				initialMemoryFileInDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				14, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // atim
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // mtim
				0x0, 0x0, 0xc2, 0xd3, 0x43, 0x6, 0x36, 0x17, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=SYMLINK_FOLLOW,path=sub/test.txt)
<== (filestat={filetype=REGULAR_FILE,size=14,mtim=1672531200000000000},errno=ESUCCESS)
`,
		},
		{
			name:           "dir under root (follow symlinks)",
			fd:             sys.FdPreopen,
			flags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:         initialMemoryDir,
			pathLen:        uint32(len(dir)),
			resultFilestat: uint32(len(dir)) + 1,
			expectedMemory: append(
				initialMemoryDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // atim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // mtim
				0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, // ctim
			),
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=SYMLINK_FOLLOW,path=sub)
<== (filestat={filetype=DIRECTORY,size=0,mtim=1640995200000000000},errno=ESUCCESS)
`,
		},
		{
			name:          "unopened FD (follow symlinks)",
			fd:            -1,
			flags:         wasip1.LOOKUP_SYMLINK_FOLLOW,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=-1,flags=SYMLINK_FOLLOW,path=)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "Fd not a directory (follow symlinks)",
			fd:             fileFD,
			flags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: 2,
			expectedErrno:  wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=4,flags=SYMLINK_FOLLOW,path=animals.txt)
<== (filestat=,errno=ENOTDIR)
`,
		},
		{
			name:           "path under root doesn't exist (follow symlinks)",
			fd:             sys.FdPreopen,
			flags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:         initialMemoryNotExists,
			pathLen:        1,
			resultFilestat: 2,
			expectedErrno:  wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=SYMLINK_FOLLOW,path=?)
<== (filestat=,errno=ENOENT)
`,
		},
		{
			name:          "path is out of memory (follow symlinks)",
			fd:            sys.FdPreopen,
			flags:         wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:        initialMemoryFile,
			pathLen:       memorySize,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=SYMLINK_FOLLOW,path=OOM(1,65536))
<== (filestat=,errno=EFAULT)
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1 (follow symlinks)",
			fd:             sys.FdPreopen,
			flags:          wasip1.LOOKUP_SYMLINK_FOLLOW,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=SYMLINK_FOLLOW,path=animals.txt)
<== (filestat=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))
			mod.Memory().Write(0, tc.memory)

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathFilestatGetName, uint64(tc.fd), uint64(tc.flags), uint64(1), uint64(tc.pathLen), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_pathFilestatSetTimes(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	file := "file"
	writeFile(t, tmpDir, file, []byte("012"))
	link := file + "-link"
	require.NoError(t, os.Symlink(joinPath(tmpDir, file), joinPath(tmpDir, link)))

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithSysWalltime().
		WithFSConfig(wazero.NewFSConfig().WithDirMount(tmpDir, "")))
	defer r.Close(testCtx)

	tests := []struct {
		name          string
		flags         uint16
		pathName      string
		mtime, atime  int64
		fstFlags      uint16
		expectedLog   string
		expectedErrno wasip1.Errno
	}{
		{
			name:  "a=omit,m=omit",
			flags: wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime: 123451, // Must be ignored.
			mtime: 1234,   // Must be ignored.
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=123451,mtim=1234,fst_flags=)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=now,m=omit",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    123451, // Must be ignored.
			mtime:    1234,   // Must be ignored.
			fstFlags: wasip1.FstflagsAtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=123451,mtim=1234,fst_flags=ATIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=omit,m=now",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    123451, // Must be ignored.
			mtime:    1234,   // Must be ignored.
			fstFlags: wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=123451,mtim=1234,fst_flags=MTIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=now,m=now",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    123451, // Must be ignored.
			mtime:    1234,   // Must be ignored.
			fstFlags: wasip1.FstflagsAtimNow | wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=123451,mtim=1234,fst_flags=ATIM_NOW|MTIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=now,m=set",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    1234, // Must be ignored.
			mtime:    55555500,
			fstFlags: wasip1.FstflagsAtimNow | wasip1.FstflagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=1234,mtim=55555500,fst_flags=ATIM_NOW|MTIM)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=set,m=now",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    55555500,
			mtime:    1234, // Must be ignored.
			fstFlags: wasip1.FstflagsAtim | wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=55555500,mtim=1234,fst_flags=ATIM|MTIM_NOW)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=set,m=omit",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    55555500,
			mtime:    1234, // Must be ignored.
			fstFlags: wasip1.FstflagsAtim,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=55555500,mtim=1234,fst_flags=ATIM)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=omit,m=set",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    1234, // Must be ignored.
			mtime:    55555500,
			fstFlags: wasip1.FstflagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=1234,mtim=55555500,fst_flags=MTIM)
<== errno=ESUCCESS
`,
		},
		{
			name:     "a=set,m=set",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			atime:    6666666600,
			mtime:    55555500,
			fstFlags: wasip1.FstflagsAtim | wasip1.FstflagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=file,atim=6666666600,mtim=55555500,fst_flags=ATIM|MTIM)
<== errno=ESUCCESS
`,
		},
		{
			name:     "not found",
			pathName: "nope",
			flags:    wasip1.LOOKUP_SYMLINK_FOLLOW,
			fstFlags: wasip1.FstflagsAtimNow, // Choose one flag to ensure an update occurs
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=SYMLINK_FOLLOW,path=nope,atim=0,mtim=0,fst_flags=ATIM_NOW)
<== errno=ENOENT
`,
			expectedErrno: wasip1.ErrnoNoent,
		},
		{
			name:     "no_symlink_follow",
			pathName: link,
			flags:    0,
			atime:    123451, // Must be ignored.
			mtime:    1234,   // Must be ignored.
			fstFlags: wasip1.FstflagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_set_times(fd=3,flags=,path=file-link,atim=123451,mtim=1234,fst_flags=MTIM_NOW)
<== errno=ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			pathName := tc.pathName
			if pathName == "" {
				pathName = file
			}
			mod.Memory().Write(0, []byte(pathName))

			fd := sys.FdPreopen
			path := 0
			pathLen := uint32(len(pathName))

			sys := mod.(*wasm.ModuleInstance).Sys
			fsc := sys.FS()

			preopen := getPreopen(t, fsc)

			var oldSt sysapi.Stat_t
			var errno experimentalsys.Errno
			if tc.expectedErrno == wasip1.ErrnoSuccess {
				oldSt, errno = preopen.Stat(pathName)
				require.EqualErrno(t, 0, errno)
			}

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathFilestatSetTimesName, uint64(fd), uint64(tc.flags),
				uint64(path), uint64(pathLen), uint64(tc.atime), uint64(tc.mtime), uint64(tc.fstFlags))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			if tc.expectedErrno != wasip1.ErrnoSuccess {
				return
			}

			newSt, errno := preopen.Stat(pathName)
			require.EqualErrno(t, 0, errno)

			if platform.CompilerSupported() {
				if tc.fstFlags&wasip1.FstflagsAtim != 0 {
					require.Equal(t, tc.atime, newSt.Atim)
				} else if tc.fstFlags&wasip1.FstflagsAtimNow != 0 {
					now := time.Now().UnixNano()
					require.True(t, newSt.Atim <= now, "expected atim %d <= now %d", newSt.Atim, now)
				} else { // omit
					require.Equal(t, oldSt.Atim, newSt.Atim)
				}
			}

			// When compiler isn't supported, we can still check mtim.
			if tc.fstFlags&wasip1.FstflagsMtim != 0 {
				require.Equal(t, tc.mtime, newSt.Mtim)
			} else if tc.fstFlags&wasip1.FstflagsMtimNow != 0 {
				now := time.Now().UnixNano()
				require.True(t, newSt.Mtim <= now, "expected mtim %d <= now %d", newSt.Mtim, now)
			} else { // omit
				require.Equal(t, oldSt.Mtim, newSt.Mtim)
			}
		})
	}
}

func Test_pathLink(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	oldDirName := "my-old-dir"
	oldDirPath := joinPath(tmpDir, oldDirName)
	mod, oldFd, log, r := requireOpenFile(t, tmpDir, oldDirName, nil, false)
	defer r.Close(testCtx)

	newDirName := "my-new-dir/sub"
	newDirPath := joinPath(tmpDir, newDirName)
	require.NoError(t, os.MkdirAll(joinPath(tmpDir, newDirName), 0o700))
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)
	newFd, errno := fsc.OpenFile(preopen, newDirName, 0o600, 0)
	require.EqualErrno(t, 0, errno)

	mem := mod.Memory()

	fileName := "file"
	err := os.WriteFile(joinPath(oldDirPath, fileName), []byte{1, 2, 3, 4}, 0o700)
	require.NoError(t, err)

	file := uint32(0xff)
	ok := mem.Write(file, []byte(fileName))
	require.True(t, ok)

	notFoundFile := uint32(0xaa)
	notFoundFileName := "nope"
	ok = mem.Write(notFoundFile, []byte(notFoundFileName))
	require.True(t, ok)

	destination := uint32(0xcc)
	destinationName := "hard-linked"
	ok = mem.Write(destination, []byte(destinationName))
	require.True(t, ok)

	destinationRealPath := joinPath(newDirPath, destinationName)

	t.Run("success", func(t *testing.T) {
		requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathLinkName,
			uint64(oldFd), 0, uint64(file), uint64(len(fileName)),
			uint64(newFd), uint64(destination), uint64(len(destinationName)))
		require.Contains(t, log.String(), wasip1.ErrnoName(wasip1.ErrnoSuccess))

		f := openFile(t, destinationRealPath, experimentalsys.O_RDONLY, 0)
		defer f.Close()

		st, errno := f.Stat()
		require.EqualErrno(t, 0, errno)
		require.False(t, st.Mode&os.ModeSymlink == os.ModeSymlink)
		require.Equal(t, uint64(2), st.Nlink)
	})

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			errno wasip1.Errno
			oldFd int32
			/* oldFlags, */ oldPath, oldPathLen uint32
			newFd                               int32
			newPath, newPathLen                 uint32
		}{
			{errno: wasip1.ErrnoBadf, oldFd: 1000},
			{errno: wasip1.ErrnoBadf, oldFd: oldFd, newFd: 1000},
			{errno: wasip1.ErrnoNotdir, oldFd: oldFd, newFd: 1},
			{errno: wasip1.ErrnoNotdir, oldFd: 1, newFd: 1},
			{errno: wasip1.ErrnoNotdir, oldFd: 1, newFd: newFd},
			{errno: wasip1.ErrnoFault, oldFd: oldFd, newFd: newFd, oldPathLen: math.MaxUint32},
			{errno: wasip1.ErrnoFault, oldFd: oldFd, newFd: newFd, newPathLen: math.MaxUint32},
			{
				errno: wasip1.ErrnoFault, oldFd: oldFd, newFd: newFd,
				oldPath: math.MaxUint32, oldPathLen: 100, newPathLen: 100,
			},
			{
				errno: wasip1.ErrnoFault, oldFd: oldFd, newFd: newFd,
				oldPath: 1, oldPathLen: 100, newPath: math.MaxUint32, newPathLen: 100,
			},
		} {
			name := wasip1.ErrnoName(tc.errno)
			t.Run(name, func(t *testing.T) {
				requireErrnoResult(t, tc.errno, mod, wasip1.PathLinkName,
					uint64(tc.oldFd), 0, uint64(tc.oldPath), uint64(tc.oldPathLen),
					uint64(tc.newFd), uint64(tc.newPath), uint64(tc.newPathLen))
				require.Contains(t, log.String(), name)
			})
		}
	})
}

func Test_pathOpen(t *testing.T) {
	dir := t.TempDir() // open before loop to ensure no locking problems.
	writeFS := sysfs.DirFS(dir)
	readFS := &sysfs.ReadFS{FS: writeFS}

	fileName := "file"
	fileContents := []byte("012")
	writeFile(t, dir, fileName, fileContents)

	appendName := "append"
	appendContents := []byte("345")
	writeFile(t, dir, appendName, appendContents)

	truncName := "trunc"
	truncContents := []byte("678")
	writeFile(t, dir, truncName, truncContents)

	dirName := "dir"
	mkdir(t, dir, dirName)

	dirFileName := joinPath(dirName, fileName)
	dirFileContents := []byte("def")
	writeFile(t, dir, dirFileName, dirFileContents)

	expectedOpenedFd := sys.FdPreopen + 1

	tests := []struct {
		name          string
		fs            experimentalsys.FS
		path          func(t *testing.T) string
		oflags        uint16
		fdflags       uint16
		rights        uint32
		expected      func(t *testing.T, fsc *sys.FSContext)
		expectedErrno wasip1.Errno
		expectedLog   string
	}{
		{
			name: "sysfs.ReadFS",
			fs:   readFS,
			path: func(*testing.T) string { return fileName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				requireContents(t, fsc, expectedOpenedFd, fileName, fileContents)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name: "sysfs.DirFS",
			fs:   writeFS,
			path: func(*testing.T) string { return fileName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				requireContents(t, fsc, expectedOpenedFd, fileName, fileContents)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "sysfs.ReadFS FD_APPEND",
			fs:            readFS,
			fdflags:       wasip1.FD_APPEND,
			path:          func(t *testing.T) (file string) { return appendName },
			expectedErrno: wasip1.ErrnoNosys,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=append,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=APPEND)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:    "sysfs.DirFS FD_APPEND",
			fs:      writeFS,
			path:    func(t *testing.T) (file string) { return appendName },
			fdflags: wasip1.FD_APPEND,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				contents := writeAndCloseFile(t, fsc, expectedOpenedFd)

				// verify the contents were appended
				b := readFile(t, dir, appendName)
				require.Equal(t, append(appendContents, contents...), b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=append,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=APPEND)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "sysfs.ReadFS O_CREAT",
			fs:            readFS,
			oflags:        wasip1.O_CREAT,
			expectedErrno: wasip1.ErrnoNosys,
			path:          func(*testing.T) string { return "creat" },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=creat,oflags=CREAT,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "sysfs.DirFS O_CREAT",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return "creat" },
			oflags: wasip1.O_CREAT,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				// expect to create a new file
				contents := writeAndCloseFile(t, fsc, expectedOpenedFd)

				// verify the contents were written
				b := readFile(t, dir, "creat")
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=creat,oflags=CREAT,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "sysfs.ReadFS O_CREAT O_TRUNC",
			fs:            readFS,
			oflags:        wasip1.O_CREAT | wasip1.O_TRUNC,
			expectedErrno: wasip1.ErrnoNosys,
			path:          func(t *testing.T) (file string) { return joinPath(dirName, "O_CREAT-O_TRUNC") },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/O_CREAT-O_TRUNC,oflags=CREAT|TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "sysfs.DirFS O_CREAT O_TRUNC",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return joinPath(dirName, "O_CREAT-O_TRUNC") },
			oflags: wasip1.O_CREAT | wasip1.O_TRUNC,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				// expect to create a new file
				contents := writeAndCloseFile(t, fsc, expectedOpenedFd)

				// verify the contents were written
				b := readFile(t, dir, joinPath(dirName, "O_CREAT-O_TRUNC"))
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/O_CREAT-O_TRUNC,oflags=CREAT|TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:   "sysfs.ReadFS O_DIRECTORY",
			fs:     readFS,
			oflags: wasip1.O_DIRECTORY,
			path:   func(*testing.T) string { return dirName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				f, ok := fsc.LookupFile(expectedOpenedFd)
				require.True(t, ok)
				isDir, errno := f.File.IsDir()
				require.EqualErrno(t, 0, errno)
				require.True(t, isDir)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:   "sysfs.DirFS O_DIRECTORY",
			fs:     writeFS,
			path:   func(*testing.T) string { return dirName },
			oflags: wasip1.O_DIRECTORY,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				f, ok := fsc.LookupFile(expectedOpenedFd)
				require.True(t, ok)
				isDir, errno := f.File.IsDir()
				require.EqualErrno(t, 0, errno)
				require.True(t, isDir)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "sysfs.ReadFS O_TRUNC",
			fs:            readFS,
			oflags:        wasip1.O_TRUNC,
			expectedErrno: wasip1.ErrnoNosys,
			path:          func(*testing.T) string { return "trunc" },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=trunc,oflags=TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "sysfs.DirFS O_TRUNC",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return "trunc" },
			oflags: wasip1.O_TRUNC,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				contents := writeAndCloseFile(t, fsc, expectedOpenedFd)

				// verify the contents were truncated
				b := readFile(t, dir, "trunc")
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=trunc,oflags=TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:   "sysfs.DirFS RIGHT_FD_READ|RIGHT_FD_WRITE",
			fs:     writeFS,
			path:   func(*testing.T) string { return fileName },
			oflags: 0,
			rights: wasip1.RIGHT_FD_READ | wasip1.RIGHT_FD_WRITE,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				requireContents(t, fsc, expectedOpenedFd, fileName, fileContents)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=,fs_rights_base=FD_READ|FD_WRITE,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "sysfs.DirFS file O_DIRECTORY RIGHTS_FD_WRITE",
			fs:            writeFS,
			path:          func(*testing.T) string { return fileName },
			oflags:        wasip1.O_DIRECTORY,
			rights:        wasip1.RIGHT_FD_WRITE,
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=DIRECTORY,fs_rights_base=FD_WRITE,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:          "sysfs.DirFS dir O_DIRECTORY RIGHTS_FD_WRITE",
			fs:            writeFS,
			path:          func(*testing.T) string { return dirName },
			oflags:        wasip1.O_DIRECTORY,
			rights:        wasip1.RIGHT_FD_WRITE,
			expectedErrno: wasip1.ErrnoIsdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=DIRECTORY,fs_rights_base=FD_WRITE,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EISDIR)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
			defer r.Close(testCtx)

			mod.(*wasm.ModuleInstance).Sys = sys.DefaultContext(tc.fs)

			pathName := tc.path(t)
			mod.Memory().Write(0, []byte(pathName))

			path := uint32(0)
			pathLen := uint32(len(pathName))
			resultOpenedFd := pathLen
			fd := sys.FdPreopen

			// TODO: dirflags is a lookupflags and it only has one bit: symlink_follow
			// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#lookupflags
			dirflags := 0

			// inherited rights aren't used
			fsRightsInheriting := uint64(0)

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathOpenName, uint64(fd), uint64(dirflags), uint64(path),
				uint64(pathLen), uint64(tc.oflags), uint64(tc.rights), fsRightsInheriting, uint64(tc.fdflags), uint64(resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			if tc.expectedErrno == wasip1.ErrnoSuccess {
				openedFd, ok := mod.Memory().ReadUint32Le(pathLen)
				require.True(t, ok)
				require.Equal(t, expectedOpenedFd, int32(openedFd))

				tc.expected(t, mod.(*wasm.ModuleInstance).Sys.FS())
			}
		})
	}
}

func writeAndCloseFile(t *testing.T, fsc *sys.FSContext, fd int32) []byte {
	contents := []byte("hello")
	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)
	_, errno := f.File.Write([]byte("hello"))
	require.EqualErrno(t, 0, errno)
	require.EqualErrno(t, 0, fsc.CloseFile(fd))
	return contents
}

func requireOpenFD(t *testing.T, mod api.Module, path string) int32 {
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	fd, errno := fsc.OpenFile(preopen, path, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)
	return fd
}

func requireContents(t *testing.T, fsc *sys.FSContext, expectedOpenedFd int32, fileName string, fileContents []byte) {
	// verify the file was actually opened
	f, ok := fsc.LookupFile(expectedOpenedFd)
	require.True(t, ok)
	require.Equal(t, fileName, f.Name)

	// verify the contents are readable
	buf := readAll(t, f.File)
	require.Equal(t, fileContents, buf)
}

func readAll(t *testing.T, f experimentalsys.File) []byte {
	st, errno := f.Stat()
	require.EqualErrno(t, 0, errno)
	buf := make([]byte, st.Size)
	_, errno = f.Read(buf)
	require.EqualErrno(t, 0, errno)
	return buf
}

func mkdir(t *testing.T, tmpDir, dir string) {
	err := os.Mkdir(joinPath(tmpDir, dir), 0o700)
	require.NoError(t, err)
}

func readFile(t *testing.T, tmpDir, file string) []byte {
	contents, err := os.ReadFile(joinPath(tmpDir, file))
	require.NoError(t, err)
	return contents
}

func writeFile(t *testing.T, tmpDir, file string, contents []byte) {
	err := os.WriteFile(joinPath(tmpDir, file), contents, 0o600)
	require.NoError(t, err)
}

func Test_pathOpen_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	file := "file"
	err := os.WriteFile(joinPath(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(joinPath(tmpDir, dir), 0o700)
	require.NoError(t, err)

	nested := "dir/nested"
	err = os.Mkdir(joinPath(tmpDir, nested), 0o700)
	require.NoError(t, err)

	nestedFile := "dir/nested/file"
	err = os.WriteFile(joinPath(tmpDir, nestedFile), []byte{}, 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName                        string
		fd                                    int32
		path, pathLen, oflags, resultOpenedFd uint32
		expectedErrno                         wasip1.Errno
		expectedLog                           string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=42,dirflags=,path=,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EBADF)
`,
		},
		{
			name:          "Fd not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=4,dirflags=,path=file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=OOM(65536,4),oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "empty path",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       0,
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EINVAL)
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=OOM(0,65537),oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "no such file exists",
			fd:            sys.FdPreopen,
			pathName:      dir,
			path:          0,
			pathLen:       uint32(len(dir)) - 1,
			expectedErrno: wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=di,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOENT)
`,
		},
		{
			name:          "trailing slash on directory",
			fd:            sys.FdPreopen,
			pathName:      nested + "/",
			path:          0,
			pathLen:       uint32(len(nested)) + 1,
			expectedErrno: wasip1.ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/nested/,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=5,errno=ESUCCESS)
`,
		},
		{
			name:          "path under preopen",
			fd:            sys.FdPreopen,
			pathName:      "../" + file,
			path:          0,
			pathLen:       uint32(len(file)) + 3,
			expectedErrno: wasip1.ErrnoPerm,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=../file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EPERM)
`,
		},
		{
			name:          "rooted path",
			fd:            sys.FdPreopen,
			pathName:      "/" + file,
			path:          0,
			pathLen:       uint32(len(file)) + 1,
			expectedErrno: wasip1.ErrnoPerm,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=/file,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EPERM)
`,
		},
		{
			name:          "trailing slash on file",
			fd:            sys.FdPreopen,
			pathName:      nestedFile + "/",
			path:          0,
			pathLen:       uint32(len(nestedFile)) + 1,
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/nested/file/,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:           "out-of-memory writing resultOpenedFd",
			fd:             sys.FdPreopen,
			pathName:       dir,
			path:           0,
			pathLen:        uint32(len(dir)),
			resultOpenedFd: mod.Memory().Size(), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "O_DIRECTORY, but not a directory",
			oflags:        uint32(wasip1.O_DIRECTORY),
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:          "oflags=directory and create invalid",
			oflags:        uint32(wasip1.O_DIRECTORY | wasip1.O_CREAT),
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=CREAT|DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EINVAL)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathOpenName, uint64(tc.fd), uint64(0), uint64(tc.path),
				uint64(tc.pathLen), uint64(tc.oflags), 0, 0, 0, uint64(tc.resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathReadlink(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	dirName := "dir"
	dirPath := joinPath(tmpDir, dirName)
	mod, dirFD, log, r := requireOpenFile(t, tmpDir, dirName, nil, false)
	defer r.Close(testCtx)

	subDirName := "sub-dir"
	require.NoError(t, os.Mkdir(joinPath(dirPath, subDirName), 0o700))

	mem := mod.Memory()

	originalFileName := "top-original-file"
	destinationPath := uint32(0x77)
	destinationPathName := "top-symlinked"
	ok := mem.Write(destinationPath, []byte(destinationPathName))
	require.True(t, ok)

	originalSubDirFileName := joinPath(subDirName, "subdir-original-file")
	destinationSubDirFileName := joinPath(subDirName, "subdir-symlinked")
	destinationSubDirPathNamePtr := uint32(0xcc)
	ok = mem.Write(destinationSubDirPathNamePtr, []byte(destinationSubDirFileName))
	require.True(t, ok)

	// Create original file and symlink to the destination.
	originalRelativePath := joinPath(dirName, originalFileName)
	err := os.WriteFile(joinPath(tmpDir, originalRelativePath), []byte{4, 3, 2, 1}, 0o700)
	require.NoError(t, err)
	err = os.Symlink(originalRelativePath, joinPath(dirPath, destinationPathName))
	require.NoError(t, err)
	originalSubDirRelativePath := joinPath(dirName, originalSubDirFileName)
	err = os.WriteFile(joinPath(tmpDir, originalSubDirRelativePath), []byte{1, 2, 3, 4}, 0o700)
	require.NoError(t, err)
	err = os.Symlink(originalSubDirRelativePath, joinPath(dirPath, destinationSubDirFileName))
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			path, pathLen uint32
			expectedBuf   string
		}{
			{
				name:        "top",
				path:        destinationPath,
				pathLen:     uint32(len(destinationPathName)),
				expectedBuf: originalRelativePath,
			},
			{
				name:        "subdir",
				path:        destinationSubDirPathNamePtr,
				pathLen:     uint32(len(destinationSubDirFileName)),
				expectedBuf: originalSubDirRelativePath,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				const buf, bufLen, resultBufused = 0x100, 0xff, 0x200
				requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathReadlinkName,
					uint64(dirFD), uint64(tc.path), uint64(tc.pathLen),
					buf, bufLen, resultBufused)
				require.Contains(t, log.String(), wasip1.ErrnoName(wasip1.ErrnoSuccess))

				size, ok := mem.ReadUint32Le(resultBufused)
				require.True(t, ok)
				actual, ok := mem.Read(buf, size)
				require.True(t, ok)
				require.Equal(t, tc.expectedBuf, string(actual))
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			name                                      string
			fd                                        int32
			path, pathLen, buf, bufLen, resultBufused uint32
			expectedErrno                             wasip1.Errno
		}{
			{expectedErrno: wasip1.ErrnoInval},
			{expectedErrno: wasip1.ErrnoInval, pathLen: 100},
			{expectedErrno: wasip1.ErrnoInval, bufLen: 100},
			{
				name:          "bufLen too short",
				expectedErrno: wasip1.ErrnoRange,
				fd:            dirFD,
				bufLen:        10,
				path:          destinationPath,
				pathLen:       uint32(len(destinationPathName)),
				buf:           0,
			},
			{
				name:          "path past memory",
				expectedErrno: wasip1.ErrnoFault,
				bufLen:        100,
				pathLen:       100,
				buf:           50,
				path:          math.MaxUint32,
			},
			{expectedErrno: wasip1.ErrnoNotdir, bufLen: 100, pathLen: 100, buf: 50, path: 50, fd: 1},
			{expectedErrno: wasip1.ErrnoBadf, bufLen: 100, pathLen: 100, buf: 50, path: 50, fd: 1000},
			{
				expectedErrno: wasip1.ErrnoNoent,
				bufLen:        100, buf: 50,
				path: destinationPath, pathLen: uint32(len(destinationPathName)) - 1,
				fd: dirFD,
			},
		} {
			name := tc.name
			if name == "" {
				name = wasip1.ErrnoName(tc.expectedErrno)
			}
			t.Run(name, func(t *testing.T) {
				requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathReadlinkName,
					uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen), uint64(tc.buf),
					uint64(tc.bufLen), uint64(tc.resultBufused))
				require.Contains(t, log.String(), wasip1.ErrnoName(tc.expectedErrno))
			})
		}
	})
}

func Test_pathRemoveDirectory(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	realPath := joinPath(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	// create the directory
	err := os.Mkdir(realPath, 0o700)
	require.NoError(t, err)

	fd := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathRemoveDirectoryName, uint64(fd), uint64(name), uint64(nameLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=wazero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the directory was removed
	_, err = os.Stat(realPath)
	require.Error(t, err)
}

func Test_pathRemoveDirectory_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	file := "file"
	err := os.WriteFile(joinPath(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dirNotEmpty := "notempty"
	dirNotEmptyPath := joinPath(tmpDir, dirNotEmpty)
	err = os.Mkdir(dirNotEmptyPath, 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(joinPath(dirNotEmptyPath, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName string
		fd             int32
		path, pathLen  uint32
		expectedErrno  wasip1.Errno
		expectedLog    string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "Fd not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=4,path=file)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       1,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "no such file exists",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file) - 1),
			expectedErrno: wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=fil)
<== errno=ENOENT
`,
		},
		{
			name:          "file not dir",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: fmt.Sprintf(`
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=file)
<== errno=%s
`, wasip1.ErrnoName(wasip1.ErrnoNotdir)),
		},
		{
			name:          "dir not empty",
			fd:            sys.FdPreopen,
			pathName:      dirNotEmpty,
			path:          0,
			pathLen:       uint32(len(dirNotEmpty)),
			expectedErrno: wasip1.ErrnoNotempty,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=notempty)
<== errno=ENOTEMPTY
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathRemoveDirectoryName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathSymlink(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	dirName := "dir"
	dirPath := joinPath(tmpDir, dirName)
	mod, fd, log, r := requireOpenFile(t, tmpDir, dirName, nil, false)
	defer r.Close(testCtx)

	mem := mod.Memory()

	fileName := "file"
	err := os.WriteFile(joinPath(dirPath, fileName), []byte{1, 2, 3, 4}, 0o700)
	require.NoError(t, err)

	file := uint32(0xff)
	ok := mem.Write(file, []byte(fileName))
	require.True(t, ok)

	notFoundFile := uint32(0xaa)
	notFoundFileName := "nope"
	ok = mem.Write(notFoundFile, []byte(notFoundFileName))
	require.True(t, ok)

	link := uint32(0xcc)
	linkName := fileName + "-link"
	ok = mem.Write(link, []byte(linkName))
	require.True(t, ok)

	successCases := []struct {
		name    string
		fd      int32
		oldPath string
		newPath string
	}{
		{
			name:    "dir",
			fd:      fd,
			oldPath: fileName,
			newPath: linkName,
		},
		{
			name:    "preopened root",
			fd:      sys.FdPreopen,
			oldPath: fileName,
			newPath: linkName,
		},
	}

	for _, tt := range successCases {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			file := uint32(0xbb)
			ok := mem.Write(file, []byte(fileName))
			require.True(t, ok)

			link := uint32(0xdd)
			ok = mem.Write(link, []byte(linkName))
			require.True(t, ok)

			requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathSymlinkName,
				uint64(file), uint64(len(tc.oldPath)), uint64(tc.fd), uint64(link), uint64(len(tc.newPath)))
			require.Contains(t, log.String(), wasip1.ErrnoName(wasip1.ErrnoSuccess))
			st, err := os.Lstat(joinPath(dirPath, linkName))
			require.NoError(t, err)
			require.Equal(t, st.Mode()&os.ModeSymlink, os.ModeSymlink)
		})
	}

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			errno               wasip1.Errno
			oldPath, oldPathLen uint32
			fd                  int32
			newPath, newPathLen uint32
		}{
			{errno: wasip1.ErrnoBadf, fd: 1000},
			{errno: wasip1.ErrnoNotdir, fd: 2},
			// Length zero buffer is not valid.
			{errno: wasip1.ErrnoInval, fd: fd},
			{errno: wasip1.ErrnoInval, oldPathLen: 100, fd: fd},
			{errno: wasip1.ErrnoInval, newPathLen: 100, fd: fd},
			// Invalid pointer to the names.
			{errno: wasip1.ErrnoFault, oldPath: math.MaxUint32, oldPathLen: 100, newPathLen: 100, fd: fd},
			{errno: wasip1.ErrnoFault, newPath: math.MaxUint32, oldPathLen: 100, newPathLen: 100, fd: fd},
			{errno: wasip1.ErrnoFault, oldPath: math.MaxUint32, newPath: math.MaxUint32, oldPathLen: 100, newPathLen: 100, fd: fd},
			// Non-existing path as source.
			{
				errno: wasip1.ErrnoInval, oldPath: notFoundFile, oldPathLen: uint32(len(notFoundFileName)),
				newPath: 0, newPathLen: 5, fd: fd,
			},
			// Linking to existing file.
			{
				errno: wasip1.ErrnoExist, oldPath: file, oldPathLen: uint32(len(fileName)),
				newPath: file, newPathLen: uint32(len(fileName)), fd: fd,
			},
		} {
			name := wasip1.ErrnoName(tc.errno)
			t.Run(name, func(t *testing.T) {
				requireErrnoResult(t, tc.errno, mod, wasip1.PathSymlinkName,
					uint64(tc.oldPath), uint64(tc.oldPathLen), uint64(tc.fd), uint64(tc.newPath), uint64(tc.newPathLen))
				require.Contains(t, log.String(), name)
			})
		}
	})
}

func Test_pathRename(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	// set up the initial memory to include the old path name starting at an offset.
	oldfd := sys.FdPreopen
	oldPathName := "wazero"
	realOldPath := joinPath(tmpDir, oldPathName)
	oldPath := uint32(0)
	oldPathLen := len(oldPathName)
	ok := mod.Memory().Write(oldPath, []byte(oldPathName))
	require.True(t, ok)

	// create the file
	err := os.WriteFile(realOldPath, []byte{}, 0o600)
	require.NoError(t, err)

	newfd := sys.FdPreopen
	newPathName := "wahzero"
	realNewPath := joinPath(tmpDir, newPathName)
	newPath := uint32(16)
	newPathLen := len(newPathName)
	ok = mod.Memory().Write(newPath, []byte(newPathName))
	require.True(t, ok)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathRenameName,
		uint64(oldfd), uint64(oldPath), uint64(oldPathLen),
		uint64(newfd), uint64(newPath), uint64(newPathLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=wazero,new_fd=3,new_path=wahzero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the file was renamed
	_, err = os.Stat(realOldPath)
	require.Error(t, err)
	_, err = os.Stat(realNewPath)
	require.NoError(t, err)
}

func Test_pathRename_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	file := "file"
	err := os.WriteFile(joinPath(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	// We have to test FD validation with a path not under test. Otherwise,
	// Windows may fail for the wrong reason, like:
	//	The process cannot access the file because it is being used by another process.
	file1 := "file1"
	err = os.WriteFile(joinPath(tmpDir, file1), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file1)

	dirNotEmpty := "notempty"
	err = os.Mkdir(joinPath(tmpDir, dirNotEmpty), 0o700)
	require.NoError(t, err)

	dir := joinPath(dirNotEmpty, "dir")
	err = os.Mkdir(joinPath(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, oldPathName, newPathName string
		oldFd                          int32
		oldPath, oldPathLen            uint32
		newFd                          int32
		newPath, newPathLen            uint32
		expectedErrno                  wasip1.Errno
		expectedLog                    string
	}{
		{
			name:          "unopened old FD",
			oldFd:         42, // arbitrary invalid fd
			newFd:         sys.FdPreopen,
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=42,old_path=,new_fd=3,new_path=)
<== errno=EBADF
`,
		},
		{
			name:          "old FD not a directory",
			oldFd:         fileFD,
			newFd:         sys.FdPreopen,
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=4,old_path=,new_fd=3,new_path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "unopened new FD",
			oldFd:         sys.FdPreopen,
			newFd:         42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=,new_fd=42,new_path=)
<== errno=EBADF
`,
		},
		{
			name:          "new FD not a directory",
			oldFd:         sys.FdPreopen,
			newFd:         fileFD,
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=,new_fd=4,new_path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading old path",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPath:       mod.Memory().Size(),
			oldPathLen:    1,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=OOM(65536,1),new_fd=3,new_path=)
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading new path",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPath:       0,
			oldPathName:   "a",
			oldPathLen:    1,
			newPath:       mod.Memory().Size(),
			newPathLen:    1,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=a,new_fd=3,new_path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading old pathLen",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPath:       0,
			oldPathLen:    mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=OOM(0,65537),new_fd=3,new_path=)
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading new pathLen",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPathName:   file,
			oldPathLen:    uint32(len(file)),
			newPath:       0,
			newPathLen:    mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=file,new_fd=3,new_path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "no such file exists",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPathName:   file,
			oldPathLen:    uint32(len(file)) - 1,
			newPath:       16,
			newPathName:   file,
			newPathLen:    uint32(len(file)),
			expectedErrno: wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=fil,new_fd=3,new_path=file)
<== errno=ENOENT
`,
		},
		{
			name:          "dir not file",
			oldFd:         sys.FdPreopen,
			newFd:         sys.FdPreopen,
			oldPathName:   file,
			oldPathLen:    uint32(len(file)),
			newPath:       16,
			newPathName:   dir,
			newPathLen:    uint32(len(dir)),
			expectedErrno: wasip1.ErrnoIsdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=file,new_fd=3,new_path=notempty/dir)
<== errno=EISDIR
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.oldPath, []byte(tc.oldPathName))
			mod.Memory().Write(tc.newPath, []byte(tc.newPathName))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathRenameName,
				uint64(tc.oldFd), uint64(tc.oldPath), uint64(tc.oldPathLen),
				uint64(tc.newFd), uint64(tc.newPath), uint64(tc.newPathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathUnlinkFile(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	realPath := joinPath(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	// create the file
	err := os.WriteFile(realPath, []byte{}, 0o600)
	require.NoError(t, err)

	fd := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathUnlinkFileName, uint64(fd), uint64(name), uint64(nameLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=wazero)
<== errno=ESUCCESS
`, "\n"+log.String())

	// ensure the file was removed
	_, err = os.Stat(realPath)
	require.Error(t, err)
}

func Test_pathUnlinkFile_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	file := "file"
	err := os.WriteFile(joinPath(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(joinPath(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName string
		fd             int32
		path, pathLen  uint32
		expectedErrno  wasip1.Errno
		expectedLog    string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: wasip1.ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "Fd not a directory",
			fd:            fileFD,
			expectedErrno: wasip1.ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=4,path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            sys.FdPreopen,
			path:          mod.Memory().Size(),
			pathLen:       1,
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=OOM(65536,1))
<== errno=EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: wasip1.ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=OOM(0,65537))
<== errno=EFAULT
`,
		},
		{
			name:          "no such file exists",
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file) - 1),
			expectedErrno: wasip1.ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=fil)
<== errno=ENOENT
`,
		},
		{
			name:          "dir not file",
			fd:            sys.FdPreopen,
			pathName:      dir,
			path:          0,
			pathLen:       uint32(len(dir)),
			expectedErrno: wasip1.ErrnoIsdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=3,path=dir)
<== errno=EISDIR
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(tc.path, []byte(tc.pathName))

			requireErrnoResult(t, tc.expectedErrno, mod, wasip1.PathUnlinkFileName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func requireOpenFile(t *testing.T, tmpDir string, pathName string, data []byte, readOnly bool) (api.Module, int32, *bytes.Buffer, api.Closer) {
	oflags := experimentalsys.O_RDWR
	if readOnly {
		oflags = experimentalsys.O_RDONLY
	}

	realPath := joinPath(tmpDir, pathName)
	if data == nil {
		oflags = experimentalsys.O_RDONLY
		require.NoError(t, os.Mkdir(realPath, 0o700))
	} else {
		require.NoError(t, os.WriteFile(realPath, data, 0o600))
	}

	fsConfig := wazero.NewFSConfig()

	if readOnly {
		fsConfig = fsConfig.WithReadOnlyDirMount(tmpDir, "/")
	} else {
		fsConfig = fsConfig.WithDirMount(tmpDir, "preopen")
	}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	fd, errno := fsc.OpenFile(preopen, pathName, oflags, 0)
	require.EqualErrno(t, 0, errno)

	return mod, fd, log, r
}

// Test_fdReaddir_dotEntryHasARealInode because wasi-testsuite requires it.
func Test_fdReaddir_dotEntryHasARealInode(t *testing.T) {
	root := t.TempDir()
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().WithDirMount(root, "/")),
	)
	defer r.Close(testCtx)

	mem := mod.Memory()

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	readDirTarget := "dir"
	mem.Write(0, []byte(readDirTarget))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathCreateDirectoryName,
		uint64(sys.FdPreopen), uint64(0), uint64(len(readDirTarget)))

	// Open the directory, before writing files!
	fd, errno := fsc.OpenFile(preopen, readDirTarget, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// get the real inode of the current directory
	st, errno := preopen.Stat(readDirTarget)
	require.EqualErrno(t, 0, errno)
	dirents := []byte{1, 0, 0, 0, 0, 0, 0, 0}         // d_next = 1
	dirents = append(dirents, u64.LeBytes(st.Ino)...) // d_ino
	dirents = append(dirents, 1, 0, 0, 0)             // d_namlen = 1 character
	dirents = append(dirents, 3, 0, 0, 0)             // d_type = directory
	dirents = append(dirents, '.')                    // name

	require.EqualErrno(t, 0, errno)
	dirents = append(dirents, 2, 0, 0, 0, 0, 0, 0, 0) // d_next = 2
	// See /RATIONALE.md for why we don't attempt to get an inode for ".."
	dirents = append(dirents, 0, 0, 0, 0, 0, 0, 0, 0) // d_ino
	dirents = append(dirents, 2, 0, 0, 0)             // d_namlen = 2 characters
	dirents = append(dirents, 3, 0, 0, 0)             // d_type = directory
	dirents = append(dirents, '.', '.')               // name

	// Try to list them!
	resultBufused := uint32(0) // where to write the amount used out of bufLen
	buf := uint32(8)           // where to start the dirents
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdReaddirName,
		uint64(fd), uint64(buf), uint64(0x2000), 0, uint64(resultBufused))

	used, _ := mem.ReadUint32Le(resultBufused)

	results, _ := mem.Read(buf, used)
	require.Equal(t, dirents, results)
}

// Test_fdReaddir_opened_file_written ensures that writing files to the already-opened directory
// is visible. This is significant on Windows.
// https://github.com/ziglang/zig/blob/2ccff5115454bab4898bae3de88f5619310bc5c1/lib/std/fs/test.zig#L156-L184
func Test_fdReaddir_opened_file_written(t *testing.T) {
	tmpDir := t.TempDir()
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().WithDirMount(tmpDir, "/")),
	)
	defer r.Close(testCtx)

	mem := mod.Memory()

	fsc := mod.(*wasm.ModuleInstance).Sys.FS()
	preopen := getPreopen(t, fsc)

	dirName := "dir"
	dirPath := joinPath(tmpDir, dirName)
	mem.Write(0, []byte(dirName))
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.PathCreateDirectoryName,
		uint64(sys.FdPreopen), uint64(0), uint64(len(dirName)))

	// Open the directory, before writing files!
	dirFD, errno := fsc.OpenFile(preopen, dirName, experimentalsys.O_RDONLY, 0)
	require.EqualErrno(t, 0, errno)

	// Then write a file to the directory.
	f := openFile(t, joinPath(dirPath, "file"), experimentalsys.O_CREAT, 0)
	defer f.Close()

	// get the real inode of the current directory
	st, errno := preopen.Stat(dirName)
	require.EqualErrno(t, 0, errno)
	dirents := []byte{1, 0, 0, 0, 0, 0, 0, 0}         // d_next = 1
	dirents = append(dirents, u64.LeBytes(st.Ino)...) // d_ino
	dirents = append(dirents, 1, 0, 0, 0)             // d_namlen = 1 character
	dirents = append(dirents, 3, 0, 0, 0)             // d_type = directory
	dirents = append(dirents, '.')                    // name

	// get the real inode of the parent directory
	st, errno = preopen.Stat(".")
	require.EqualErrno(t, 0, errno)
	dirents = append(dirents, 2, 0, 0, 0, 0, 0, 0, 0) // d_next = 2
	// See /RATIONALE.md for why we don't attempt to get an inode for ".."
	dirents = append(dirents, 0, 0, 0, 0, 0, 0, 0, 0) // d_ino
	dirents = append(dirents, 2, 0, 0, 0)             // d_namlen = 2 characters
	dirents = append(dirents, 3, 0, 0, 0)             // d_type = directory
	dirents = append(dirents, '.', '.')               // name

	// get the real inode of the file
	st, errno = f.Stat()
	require.EqualErrno(t, 0, errno)
	dirents = append(dirents, 3, 0, 0, 0, 0, 0, 0, 0) // d_next = 3
	dirents = append(dirents, u64.LeBytes(st.Ino)...) // d_ino
	dirents = append(dirents, 4, 0, 0, 0)             // d_namlen = 4 characters
	dirents = append(dirents, 4, 0, 0, 0)             // d_type = regular_file
	dirents = append(dirents, 'f', 'i', 'l', 'e')     // name

	// Try to list them!
	resultBufused := uint32(0) // where to write the amount used out of bufLen
	buf := uint32(8)           // where to start the dirents
	requireErrnoResult(t, wasip1.ErrnoSuccess, mod, wasip1.FdReaddirName,
		uint64(dirFD), uint64(buf), uint64(0x2000), 0, uint64(resultBufused))

	used, _ := mem.ReadUint32Le(resultBufused)

	results, _ := mem.Read(buf, used)
	require.Equal(t, dirents, results)
}

// joinPath avoids us having to rename fields just to avoid conflict with the
// path package.
func joinPath(dirName, baseName string) string {
	return path.Join(dirName, baseName)
}

func openFile(t *testing.T, path string, flag experimentalsys.Oflag, perm fs.FileMode) experimentalsys.File {
	f, errno := sysfs.OpenOSFile(path, flag, perm)
	require.EqualErrno(t, 0, errno)
	return f
}
