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
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/fstest"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	"github.com/tetratelabs/wazero/internal/testing/require"
	. "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_fdAdvise(t *testing.T) {
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	requireErrnoResult(t, ErrnoSuccess, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceNormal))
	requireErrnoResult(t, ErrnoSuccess, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceSequential))
	requireErrnoResult(t, ErrnoSuccess, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceRandom))
	requireErrnoResult(t, ErrnoSuccess, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceWillNeed))
	requireErrnoResult(t, ErrnoSuccess, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceDontNeed))
	requireErrnoResult(t, ErrnoSuccess, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceNoReuse))
	requireErrnoResult(t, ErrnoInval, mod, FdAdviseName, uint64(3), 0, 0, uint64(FdAdviceNoReuse+1))
	requireErrnoResult(t, ErrnoBadf, mod, FdAdviseName, uint64(1111111), 0, 0, uint64(FdAdviceNoReuse+1))
}

// Test_fdAllocate only tests it is stubbed for GrainLang per #271
func Test_fdAllocate(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	const fileName = "file.txt"

	// Create the target file.
	realPath := path.Join(tmpDir, fileName)
	require.NoError(t, os.WriteFile(realPath, []byte("0123456789"), 0o600))

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(
		wazero.NewFSConfig().WithDirMount(tmpDir, "/"),
	))
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()
	defer r.Close(testCtx)

	fd, err := fsc.OpenFile(preopen, fileName, os.O_RDWR, 0)
	require.NoError(t, err)

	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)

	requireSizeEqual := func(exp int64) {
		var st platform.Stat_t
		require.NoError(t, f.Stat(&st))
		require.Equal(t, exp, st.Size)
	}

	t.Run("errors", func(t *testing.T) {
		requireErrnoResult(t, ErrnoBadf, mod, FdAllocateName, uint64(12345), 0, 0)
		minusOne := int64(-1)
		requireErrnoResult(t, ErrnoInval, mod, FdAllocateName, uint64(fd), uint64(minusOne), uint64(minusOne))
		requireErrnoResult(t, ErrnoInval, mod, FdAllocateName, uint64(fd), 0, uint64(minusOne))
		requireErrnoResult(t, ErrnoInval, mod, FdAllocateName, uint64(fd), uint64(minusOne), 0)
	})

	t.Run("do not change size", func(t *testing.T) {
		for _, tc := range []struct{ offset, length uint64 }{
			{offset: 0, length: 10},
			{offset: 5, length: 5},
			{offset: 4, length: 0},
			{offset: 10, length: 0},
		} {
			// This shouldn't change the size.
			requireErrnoResult(t, ErrnoSuccess, mod, FdAllocateName,
				uint64(fd), tc.offset, tc.length)
			requireSizeEqual(10)
		}
	})

	t.Run("increase", func(t *testing.T) {
		// 10 + 10 > the current size -> increase the size.
		requireErrnoResult(t, ErrnoSuccess, mod, FdAllocateName,
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

func Test_fdClose(t *testing.T) {
	// fd_close needs to close an open file descriptor. Open two files so that we can tell which is closed.
	path1, path2 := "dir/-", "dir/a-"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fdToClose, err := fsc.OpenFile(preopen, path1, os.O_RDONLY, 0)
	require.NoError(t, err)

	fdToKeep, err := fsc.OpenFile(preopen, path2, os.O_RDONLY, 0)
	require.NoError(t, err)

	// Close
	requireErrnoResult(t, ErrnoSuccess, mod, FdCloseName, uint64(fdToClose))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=4)
<== errno=ESUCCESS
`, "\n"+log.String())

	// Verify fdToClose is closed and removed from the opened FDs.
	_, ok := fsc.LookupFile(fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.LookupFile(fdToKeep)
	require.True(t, ok)

	log.Reset()
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		requireErrnoResult(t, ErrnoBadf, mod, FdCloseName, uint64(42)) // 42 is an arbitrary invalid FD
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=42)
<== errno=EBADF
`, "\n"+log.String())
	})
	log.Reset()
	t.Run("ErrnoNotsup for a preopen", func(t *testing.T) {
		requireErrnoResult(t, ErrnoNotsup, mod, FdCloseName, uint64(sys.FdPreopen))
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=3)
<== errno=ENOTSUP
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
		fd            uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_datasync(fd=42)
<== errno=EBADF
`,
		},
		{
			name:          "valid fd",
			fd:            fd,
			expectedErrno: ErrnoSuccess,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdDatasyncName, uint64(tc.fd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdFdstatGet(t *testing.T) {
	file, dir := "animals.txt", "sub"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fileFD, err := fsc.OpenFile(preopen, file, os.O_RDONLY, 0)
	require.NoError(t, err)

	dirFD, err := fsc.OpenFile(preopen, dir, os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name             string
		fd, resultFdstat uint32
		expectedMemory   []byte
		expectedErrno    Errno
		expectedLog      string
	}{
		{
			name: "stdin",
			fd:   sys.FdStdin,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=0)
<== (stat={filetype=BLOCK_DEVICE,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stdout",
			fd:   sys.FdStdout,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				1, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=1)
<== (stat={filetype=BLOCK_DEVICE,fdflags=APPEND,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "stderr",
			fd:   sys.FdStderr,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				1, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=2)
<== (stat={filetype=BLOCK_DEVICE,fdflags=APPEND,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "root",
			fd:   sys.FdPreopen,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=3)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "file",
			fd:   fileFD,
			expectedMemory: []byte{
				4, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=4)
<== (stat={filetype=REGULAR_FILE,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name: "dir",
			fd:   dirFD,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5)
<== (stat={filetype=DIRECTORY,fdflags=,fs_rights_base=,fs_rights_inheriting=},errno=ESUCCESS)
`,
		},
		{
			name:          "bad FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=-1)
<== (stat=,errno=EBADF)
`,
		},
		{
			name:          "resultFdstat exceeds the maximum valid address by 1",
			fd:            dirFD,
			resultFdstat:  memorySize - 24 + 1,
			expectedErrno: ErrnoFault,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdFdstatGetName, uint64(tc.fd), uint64(tc.resultFdstat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdFdstatSetFlags(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	const fileName = "file.txt"

	// Create the target file.
	realPath := path.Join(tmpDir, fileName)
	require.NoError(t, os.WriteFile(realPath, []byte("0123456789"), 0o600))

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(wazero.NewFSConfig().
		WithDirMount(tmpDir, "/")))
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()
	defer r.Close(testCtx)

	// First, open it with O_APPEND.
	fd, err := fsc.OpenFile(preopen, fileName, os.O_RDWR|os.O_APPEND, 0)
	require.NoError(t, err)

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

		requireErrnoResult(t, ErrnoSuccess, mod, FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2)
<== (nwritten=6,errno=ESUCCESS)
`, "\n"+log.String())
		log.Reset()
	}

	requireFileContent := func(exp string) {
		buf, err := os.ReadFile(path.Join(tmpDir, fileName))
		require.NoError(t, err)
		require.Equal(t, exp, string(buf))
	}

	// with O_APPEND flag, the data is appended to buffer.
	writeWazero()
	requireFileContent("0123456789" + "wazero")

	// Let's remove O_APPEND.
	requireErrnoResult(t, ErrnoSuccess, mod, FdFdstatSetFlagsName, uint64(fd), uint64(0))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=4,flags=0)
<== errno=ESUCCESS
`, "\n"+log.String())
	log.Reset()

	// Without O_APPEND flag, the data is written at the beginning.
	writeWazero()
	requireFileContent("wazero6789" + "wazero")

	// Restore the O_APPEND flag.
	requireErrnoResult(t, ErrnoSuccess, mod, FdFdstatSetFlagsName, uint64(fd), uint64(FD_APPEND))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=4,flags=1)
<== errno=ESUCCESS
`, "\n"+log.String())
	log.Reset()

	// with O_APPEND flag, the data is appended to buffer.
	writeWazero()
	requireFileContent("wazero6789" + "wazero" + "wazero")

	t.Run("errors", func(t *testing.T) {
		requireErrnoResult(t, ErrnoInval, mod, FdFdstatSetFlagsName, uint64(fd), uint64(FD_DSYNC))
		requireErrnoResult(t, ErrnoInval, mod, FdFdstatSetFlagsName, uint64(fd), uint64(FD_NONBLOCK))
		requireErrnoResult(t, ErrnoInval, mod, FdFdstatSetFlagsName, uint64(fd), uint64(FD_RSYNC))
		requireErrnoResult(t, ErrnoInval, mod, FdFdstatSetFlagsName, uint64(fd), uint64(FD_SYNC))
		requireErrnoResult(t, ErrnoBadf, mod, FdFdstatSetFlagsName, uint64(12345), uint64(FD_APPEND))
		requireErrnoResult(t, ErrnoIsdir, mod, FdFdstatSetFlagsName, uint64(3) /* preopen */, uint64(FD_APPEND))
	})
}

// Test_fdFdstatSetRights only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetRights(t *testing.T) {
	log := requireErrnoNosys(t, FdFdstatSetRightsName, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_fdstat_set_rights(fd=0,fs_rights_base=,fs_rights_inheriting=)
<-- errno=ENOSYS
`, log)
}

func Test_fdFilestatGet(t *testing.T) {
	file, dir := "animals.txt", "sub"
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fileFD, err := fsc.OpenFile(preopen, file, os.O_RDONLY, 0)
	require.NoError(t, err)

	dirFD, err := fsc.OpenFile(preopen, dir, os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name               string
		fd, resultFilestat uint32
		expectedMemory     []byte
		expectedErrno      Errno
		expectedLog        string
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
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_get(fd=-1)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             dirFD,
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  ErrnoFault,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdFilestatGetName, uint64(tc.fd), uint64(tc.resultFilestat))
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
		size                     uint32
		content, expectedContent []byte
		expectedLog              string
		expectedErrno            Errno
	}{
		{
			name:            "badf",
			content:         []byte("badf"),
			expectedContent: []byte("badf"),
			expectedErrno:   ErrnoBadf,
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
			expectedErrno:   ErrnoSuccess,
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
			expectedErrno:   ErrnoSuccess,
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
			expectedErrno:   ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_size(fd=4,size=106)
<== errno=ESUCCESS
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
			requireErrnoResult(t, tc.expectedErrno, mod, FdFilestatSetSizeName, uint64(fd), uint64(tc.size))

			actual, err := os.ReadFile(path.Join(tmpDir, filepath))
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
		expectedErrno Errno
	}{
		{
			name:          "badf",
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=5,atim=0,mtim=0,fst_flags=0)
<== errno=EBADF
`,
		},
		{
			name:          "a=omit,m=omit",
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			expectedErrno: ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=0)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=now,m=omit",
			expectedErrno: ErrnoSuccess,
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			flags:         FileStatAdjustFlagsAtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=2)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=omit,m=now",
			expectedErrno: ErrnoSuccess,
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			flags:         FileStatAdjustFlagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=8)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=now,m=now",
			expectedErrno: ErrnoSuccess,
			mtime:         1234,   // Must be ignored.
			atime:         123451, // Must be ignored.
			flags:         FileStatAdjustFlagsAtimNow | FileStatAdjustFlagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=123451,mtim=1234,fst_flags=10)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=set,m=omit",
			expectedErrno: ErrnoSuccess,
			mtime:         1234, // Must be ignored.
			atime:         55555500,
			flags:         FileStatAdjustFlagsAtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=55555500,mtim=1234,fst_flags=1)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=set,m=now",
			expectedErrno: ErrnoSuccess,
			mtime:         1234, // Must be ignored.
			atime:         55555500,
			flags:         FileStatAdjustFlagsAtim | FileStatAdjustFlagsMtimNow,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=55555500,mtim=1234,fst_flags=9)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=omit,m=set",
			expectedErrno: ErrnoSuccess,
			mtime:         55555500,
			atime:         1234, // Must be ignored.
			flags:         FileStatAdjustFlagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=1234,mtim=55555500,fst_flags=4)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=now,m=set",
			expectedErrno: ErrnoSuccess,
			mtime:         55555500,
			atime:         1234, // Must be ignored.
			flags:         FileStatAdjustFlagsAtimNow | FileStatAdjustFlagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=1234,mtim=55555500,fst_flags=6)
<== errno=ESUCCESS
`,
		},
		{
			name:          "a=set,m=set",
			expectedErrno: ErrnoSuccess,
			mtime:         55555500,
			atime:         6666666600,
			flags:         FileStatAdjustFlagsAtim | FileStatAdjustFlagsMtim,
			expectedLog: `
==> wasi_snapshot_preview1.fd_filestat_set_times(fd=4,atim=6666666600,mtim=55555500,fst_flags=5)
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

			sys := mod.(*wasm.CallContext).Sys
			fsc := sys.FS()

			paramFd := fd
			if filepath == "badf" {
				paramFd = fd + 1
			}

			f, ok := fsc.LookupFile(fd)
			require.True(t, ok)

			var st platform.Stat_t
			require.NoError(t, f.Stat(&st))
			prevAtime, prevMtime := st.Atim, st.Mtim

			requireErrnoResult(t, tc.expectedErrno, mod, FdFilestatSetTimesName,
				uint64(paramFd), uint64(tc.atime), uint64(tc.mtime),
				uint64(tc.flags),
			)

			if tc.expectedErrno == ErrnoSuccess {
				f, ok := fsc.LookupFile(fd)
				require.True(t, ok)

				var st platform.Stat_t
				require.NoError(t, f.Stat(&st))
				if tc.flags&FileStatAdjustFlagsAtim != 0 {
					require.Equal(t, tc.atime, st.Atim)
				} else if tc.flags&FileStatAdjustFlagsAtimNow != 0 {
					require.True(t, (sys.WalltimeNanos()-st.Atim) < time.Second.Nanoseconds())
				} else {
					require.Equal(t, prevAtime, st.Atim)
				}
				if tc.flags&FileStatAdjustFlagsMtim != 0 {
					require.Equal(t, tc.mtime, st.Mtim)
				} else if tc.flags&FileStatAdjustFlagsMtimNow != 0 {
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

			requireErrnoResult(t, ErrnoSuccess, mod, FdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(tc.offset), uint64(resultNread))
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

	requireErrnoResult(t, ErrnoSuccess, mod, FdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), 2, uint64(resultNread))
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

	requireErrnoResult(t, ErrnoSuccess, mod, FdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
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
		name                             string
		fd, iovs, iovsCount, resultNread uint32
		offset                           int64
		memory                           []byte
		expectedErrno                    Errno
		expectedLog                      string
	}{
		{
			name:          "invalid fd",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nread validation
			expectedErrno: ErrnoBadf,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoIo,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdPreadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount), uint64(tc.offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatGet(t *testing.T) {
	fsConfig := wazero.NewFSConfig().WithDirMount(t.TempDir(), "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)
	dirFD := sys.FdPreopen

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

	requireErrnoResult(t, ErrnoSuccess, mod, FdPrestatGetName, uint64(dirFD), uint64(resultPrestat))
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
		fd            uint32
		resultPrestat uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid FD
			resultPrestat: 0,  // valid offset
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=42)
<== (prestat=,errno=EBADF)
`,
		},
		{
			name:          "not pre-opened FD",
			fd:            dirFD,
			resultPrestat: 0, // valid offset
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=4)
<== (prestat=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory resultPrestat",
			fd:            sys.FdPreopen,
			resultPrestat: memorySize,
			expectedErrno: ErrnoFault,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdPrestatGetName, uint64(tc.fd), uint64(tc.resultPrestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatDirName(t *testing.T) {
	fsConfig := wazero.NewFSConfig().WithDirMount(t.TempDir(), "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)
	dirFD := sys.FdPreopen

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(0) // shorter than len("/") to prove truncation is ok
	expectedMemory := []byte{
		'?', '?', '?', '?',
	}

	maskMemory(t, mod, len(expectedMemory))

	requireErrnoResult(t, ErrnoSuccess, mod, FdPrestatDirNameName, uint64(dirFD), uint64(path), uint64(pathLen))
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
		fd            uint32
		path          uint32
		pathLen       uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "out-of-memory path",
			fd:            sys.FdPreopen,
			path:          memorySize,
			pathLen:       pathLen,
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoNametoolong,
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
			expectedErrno: ErrnoBadf,
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
			expectedErrno: ErrnoBadf,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdPrestatDirNameName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
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

			requireErrnoResult(t, ErrnoSuccess, mod, FdPwriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(tc.offset), uint64(resultNwritten))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)

			// Ensure the contents were really written
			b, err := os.ReadFile(path.Join(tmpDir, pathName))
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
	requireErrnoResult(t, ErrnoSuccess, mod, FdPwriteName, uint64(fd), uint64(iovs), uint64(iovsCount), 3, uint64(resultNwritten))
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

	requireErrnoResult(t, ErrnoSuccess, mod, FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
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
	b, err := os.ReadFile(path.Join(tmpDir, pathName))
	require.NoError(t, err)
	require.Equal(t, "wazero", string(b))
}

func Test_fdPwrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, []byte{}, false)
	defer r.Close(testCtx)

	tests := []struct {
		name                                string
		fd, iovs, iovsCount, resultNwritten uint32
		offset                              int64
		memory                              []byte
		expectedErrno                       Errno
		expectedLog                         string
	}{
		{
			name:          "invalid fd",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nwritten validation
			expectedErrno: ErrnoBadf,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoIo,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdPwriteName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount), uint64(tc.offset), uint64(tc.resultNwritten+offset))
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
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',        // resultNread is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, mod, len(expectedMemory))

	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrnoResult(t, ErrnoSuccess, mod, FdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=2)
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
		name                             string
		fd, iovs, iovsCount, resultNread uint32
		memory                           []byte
		expectedErrno                    Errno
		expectedLog                      string
	}{
		{
			name:          "invalid fd",
			fd:            42,                         // arbitrary invalid fd
			memory:        []byte{'?', '?', '?', '?'}, // pass result.nread validation
			expectedErrno: ErrnoBadf,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdReadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

var (
	testDirEntries = func() []fs.DirEntry {
		entries, err := fstest.FS.ReadDir("dir")
		if err != nil {
			panic(err)
		}
		d, err := fstest.FS.Open("dir")
		if err != nil {
			panic(err)
		}
		defer d.Close()
		dots, err := sys.DotEntries(d)
		if err != nil {
			panic(err)
		}
		return append(dots, entries...)
	}()

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

	dirents = append(append(append(append(direntDot, direntDotDot...), dirent1...), dirent2...), dirent3...)
)

func Test_fdReaddir(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fd, err := fsc.OpenFile(preopen, "dir", os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name            string
		dir             func() *sys.FileEntry
		bufLen          uint32
		cookie          int64
		expectedMem     []byte
		expectedMemSize int
		expectedBufused uint32
		expectedReadDir *sys.ReadDir
	}{
		{
			name: "empty dir",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("emptydir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          DirentSize + 1, // size of one entry
			cookie:          0,
			expectedBufused: DirentSize + 1, // one dot entry
			expectedMem:     direntDot,
			expectedReadDir: &sys.ReadDir{
				CountRead: 2,
				Entries:   testDirEntries[0:2], // dot and dot-dot
			},
		},
		{
			name: "full read",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          4096,
			cookie:          0,
			expectedBufused: 129, // length of all entries
			expectedMem:     dirents,
			expectedReadDir: &sys.ReadDir{
				CountRead: 5,
				Entries:   testDirEntries,
			},
		},
		{
			name: "can't read name",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          DirentSize, // length is long enough for first, but not the name.
			cookie:          0,
			expectedBufused: DirentSize,             // == bufLen which is the size of the dirent
			expectedMem:     direntDot[:DirentSize], // header without name
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[0:3],
			},
		},
		{
			name: "read exactly first",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)

				return &sys.FileEntry{File: dir}
			},
			bufLen:          25, // length is long enough for first + the name, but not more.
			cookie:          0,
			expectedBufused: 25, // length to read exactly first.
			expectedMem:     direntDot,
			expectedReadDir: &sys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[0:3],
			},
		},
		{
			name: "read exactly second",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 3,
						Entries:   append(testDirEntries[0:2], entry...),
					},
				}
			},
			bufLen:          27, // length is long enough for exactly second.
			cookie:          1,  // d_next of first
			expectedBufused: 27, // length to read exactly second.
			expectedMem:     direntDotDot,
			expectedReadDir: &sys.ReadDir{
				CountRead: 4,
				Entries:   testDirEntries[1:4],
			},
		},
		{
			name: "read second and a little more",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 3,
						Entries:   append(testDirEntries[0:2], entry...),
					},
				}
			},
			bufLen:          30, // length is longer than the second entry, but not long enough for a header.
			cookie:          1,  // d_next of first
			expectedBufused: 30, // length to read some more, but not enough for a header, so buf was exhausted.
			expectedMem:     direntDotDot,
			expectedMemSize: len(direntDotDot), // we do not want to compare the full buffer since we don't know what the leftover 4 bytes will contain.
			expectedReadDir: &sys.ReadDir{
				CountRead: 4,
				Entries:   testDirEntries[1:4],
			},
		},
		{
			name: "read second and header of third",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 3,
						Entries:   append(testDirEntries[0:2], entry...),
					},
				}
			},
			bufLen:          50, // length is longer than the second entry + enough for the header of third.
			cookie:          1,  // d_next of first
			expectedBufused: 50, // length to read exactly second and the header of third.
			expectedMem:     append(direntDotDot, dirent1[0:24]...),
			expectedReadDir: &sys.ReadDir{
				CountRead: 5,
				Entries:   testDirEntries[1:5],
			},
		},
		{
			name: "read second and third",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 3,
						Entries:   append(testDirEntries[0:2], entry...),
					},
				}
			},
			bufLen:          53, // length is long enough for second and third.
			cookie:          1,  // d_next of first
			expectedBufused: 53, // length to read exactly one second and third.
			expectedMem:     append(direntDotDot, dirent1...),
			expectedReadDir: &sys.ReadDir{
				CountRead: 5,
				Entries:   testDirEntries[1:5],
			},
		},
		{
			name: "read exactly third",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				two, err := dir.(fs.ReadDirFile).ReadDir(2)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 4,
						Entries:   append(testDirEntries[0:2], two[0:]...),
					},
				}
			},
			bufLen:          27, // length is long enough for exactly third.
			cookie:          2,  // d_next of second.
			expectedBufused: 27, // length to read exactly third.
			expectedMem:     dirent1,
			expectedReadDir: &sys.ReadDir{
				CountRead: 5,
				Entries:   testDirEntries[2:],
			},
		},
		{
			name: "read third and beyond",
			dir: func() *sys.FileEntry {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				two, err := dir.(fs.ReadDirFile).ReadDir(2)
				require.NoError(t, err)

				return &sys.FileEntry{
					File: dir,
					ReadDir: &sys.ReadDir{
						CountRead: 4,
						Entries:   append(testDirEntries[0:2], two[0:]...),
					},
				}
			},
			bufLen:          300, // length is long enough for third and more
			cookie:          2,   // d_next of second.
			expectedBufused: 78,  // length to read the rest
			expectedMem:     append(dirent1, dirent2...),
			expectedReadDir: &sys.ReadDir{
				CountRead: 5,
				Entries:   testDirEntries[2:],
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Assign the state we are testing
			file, ok := fsc.LookupFile(fd)
			require.True(t, ok)
			dir := tc.dir()
			defer dir.File.Close()

			file.File = dir.File
			file.ReadDir = dir.ReadDir

			maskMemory(t, mod, int(tc.bufLen))

			resultBufused := uint32(0) // where to write the amount used out of bufLen
			buf := uint32(8)           // where to start the dirents
			requireErrnoResult(t, ErrnoSuccess, mod, FdReaddirName,
				uint64(fd), uint64(buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(resultBufused))

			// read back the bufused and compare memory against it
			bufUsed, ok := mod.Memory().ReadUint32Le(resultBufused)
			require.True(t, ok)
			require.Equal(t, tc.expectedBufused, bufUsed)

			mem, ok := mod.Memory().Read(buf, bufUsed)
			require.True(t, ok)

			if tc.expectedMem != nil {
				if tc.expectedMemSize == 0 {
					tc.expectedMemSize = len(tc.expectedMem)
				}
				require.Equal(t, tc.expectedMem, mem[:tc.expectedMemSize])
			}

			require.Equal(t, tc.expectedReadDir, file.ReadDir)
		})
	}
}

func Test_fdReaddir_Rewind(t *testing.T) {
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, err := fsc.OpenFile(fsc.RootFS(), "dir", os.O_RDONLY, 0)
	require.NoError(t, err)

	mem := mod.Memory()
	const resultBufUsed, buf, bufSize = 0, 8, 200
	read := func(cookie, bufSize uint64) (bufUsed uint32) {
		requireErrnoResult(t, ErrnoSuccess, mod, FdReaddirName,
			uint64(fd), buf, bufSize, cookie, uint64(resultBufUsed))

		bufUsed, ok := mem.ReadUint32Le(resultBufUsed)
		require.True(t, ok)
		return bufUsed
	}

	cookie := uint64(0)
	// Initial read.
	initialBufUsed := read(cookie, bufSize)
	// Ensure that all is read.
	require.Equal(t, len(dirents), int(initialBufUsed))
	resultBuf, ok := mem.Read(buf, initialBufUsed)
	require.True(t, ok)
	require.Equal(t, dirents, resultBuf)

	// Mask the result.
	for i := range resultBuf {
		resultBuf[i] = '?'
	}

	// Advance the cookie beyond the existing entries.
	cookie += 5
	// Nothing to read from, so bufUsed must be zero.
	require.Zero(t, int(read(cookie, bufSize)))

	// Ensure buffer is intact.
	for i := range resultBuf {
		require.Equal(t, byte('?'), resultBuf[i])
	}

	// Here, we rewind the directory by setting cookie=0 on the same file descriptor.
	cookie = 0
	usedAfterRewind := read(cookie, bufSize)
	// Ensure that all is read.
	require.Equal(t, len(dirents), int(usedAfterRewind))
	resultBuf, ok = mem.Read(buf, usedAfterRewind)
	require.True(t, ok)
	require.Equal(t, dirents, resultBuf)
}

func Test_fdReaddir_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.FS))
	defer r.Close(testCtx)
	memLen := mod.Memory().Size()

	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fileFD, err := fsc.OpenFile(preopen, "animals.txt", os.O_RDONLY, 0)
	require.NoError(t, err)

	dirFD, err := fsc.OpenFile(preopen, "dir", os.O_RDONLY, 0)
	require.NoError(t, err)

	tests := []struct {
		name                           string
		dir                            func() *sys.FileEntry
		fd, buf, bufLen, resultBufused uint32
		cookie                         int64
		readDir                        *sys.ReadDir
		expectedErrno                  Errno
		expectedLog                    string
	}{
		{
			name:          "out-of-memory reading buf",
			fd:            dirFD,
			buf:           memLen,
			bufLen:        1000,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=65536,buf_len=1000,cookie=0,result.bufused=0)
<== errno=EFAULT
`,
		},
		{
			name: "invalid fd",
			fd:   42,                    // arbitrary invalid fd
			buf:  0, bufLen: DirentSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=42,buf=0,buf_len=24,cookie=0,result.bufused=1000)
<== errno=EBADF
`,
		},
		{
			name: "not a dir",
			fd:   fileFD,
			buf:  0, bufLen: DirentSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=24,cookie=0,result.bufused=1000)
<== errno=EBADF
`,
		},
		{
			name:          "out-of-memory reading bufLen",
			fd:            dirFD,
			buf:           memLen - 1,
			bufLen:        1000,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=65535,buf_len=1000,cookie=0,result.bufused=0)
<== errno=EFAULT
`,
		},
		{
			name: "bufLen must be enough to write a struct",
			fd:   dirFD,
			buf:  0, bufLen: 1,
			resultBufused: 1000,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1,cookie=0,result.bufused=1000)
<== errno=EINVAL
`,
		},
		{
			name: "cookie invalid when no prior state",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        1,
			resultBufused: 2000,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1000,cookie=1,result.bufused=2000)
<== errno=EINVAL
`,
		},
		{
			name: "negative cookie invalid",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        -1,
			readDir:       &sys.ReadDir{CountRead: 1},
			resultBufused: 2000,
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=1000,cookie=-1,result.bufused=2000)
<== errno=EINVAL
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Reset the directory so that tests don't taint each other.
			if file, ok := fsc.LookupFile(tc.fd); ok && tc.fd == dirFD {
				dir, err := fstest.FS.Open("dir")
				require.NoError(t, err)
				defer dir.Close()

				file.File = dir
				file.ReadDir = nil
			}

			requireErrnoResult(t, tc.expectedErrno, mod, FdReaddirName,
				uint64(tc.fd), uint64(tc.buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(tc.resultBufused))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdRenumber(t *testing.T) {
	const preopenFd, fileFd, dirFd = 3, 4, 5

	tests := []struct {
		name          string
		from, to      uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "from=preopen",
			from:          preopenFd,
			to:            dirFd,
			expectedErrno: ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=3,to=5)
<== errno=ENOTSUP
`,
		},
		{
			name:          "to=preopen",
			from:          dirFd,
			to:            3,
			expectedErrno: ErrnoNotsup,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=5,to=3)
<== errno=ENOTSUP
`,
		},
		{
			name:          "file to dir",
			from:          fileFd,
			to:            dirFd,
			expectedErrno: ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=4,to=5)
<== errno=ESUCCESS
`,
		},
		{
			name:          "dir to file",
			from:          dirFd,
			to:            fileFd,
			expectedErrno: ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=5,to=4)
<== errno=ESUCCESS
`,
		},
		{
			name:          "dir to any",
			from:          dirFd,
			to:            12345,
			expectedErrno: ErrnoSuccess,
			expectedLog: `
==> wasi_snapshot_preview1.fd_renumber(fd=5,to=12345)
<== errno=ESUCCESS
`,
		},
		{
			name:          "file to any",
			from:          fileFd,
			to:            54,
			expectedErrno: ErrnoSuccess,
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

			fsc := mod.(*wasm.CallContext).Sys.FS()
			preopen := fsc.RootFS()

			// Sanity check of the file descriptor assignment.
			fileFdAssigned, err := fsc.OpenFile(preopen, "animals.txt", os.O_RDONLY, 0)
			require.NoError(t, err)
			require.Equal(t, uint32(fileFd), fileFdAssigned)

			dirFdAssigned, err := fsc.OpenFile(preopen, "dir", os.O_RDONLY, 0)
			require.NoError(t, err)
			require.Equal(t, uint32(dirFd), dirFdAssigned)

			requireErrnoResult(t, tc.expectedErrno, mod, FdRenumberName, uint64(tc.from), uint64(tc.to))
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
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=4,whence=0,4557430888798830399)
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
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=1,whence=1,4557430888798830399)
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
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=-1,whence=2,4557430888798830399)
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
			fsc := mod.(*wasm.CallContext).Sys.FS()
			f, ok := fsc.LookupFile(fd)
			require.True(t, ok)
			seeker := f.File.(io.Seeker)

			// set the initial offset of the file to 1
			offset, err := seeker.Seek(1, io.SeekStart)
			require.NoError(t, err)
			require.Equal(t, int64(1), offset)

			requireErrnoResult(t, ErrnoSuccess, mod, FdSeekName, uint64(fd), uint64(tc.offset), uint64(tc.whence), uint64(resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)

			offset, err = seeker.Seek(0, io.SeekCurrent)
			require.NoError(t, err)
			require.Equal(t, tc.expectedOffset, offset) // test that the offset of file is actually updated.
		})
	}
}

func Test_fdSeek_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), false)
	defer r.Close(testCtx)

	fsc := mod.(*wasm.CallContext).Sys.FS()
	require.NoError(t, fsc.RootFS().Mkdir("dir", 0o0700))
	dirFD := requireOpenFD(t, mod, "dir")

	memorySize := mod.Memory().Size()

	tests := []struct {
		name                    string
		fd                      uint32
		offset                  uint64
		whence, resultNewoffset uint32
		expectedErrno           Errno
		expectedLog             string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=42,offset=0,whence=0,0)
<== (newoffset=,errno=EBADF)
`,
		},
		{
			name:          "invalid whence",
			fd:            fd,
			whence:        3, // invalid whence, the largest whence io.SeekEnd(2) + 1
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=3,0)
<== (newoffset=,errno=EINVAL)
`,
		},
		{
			name:          "dir not file",
			fd:            dirFD,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=5,offset=0,whence=0,0)
<== (newoffset=,errno=EBADF)
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fd,
			resultNewoffset: memorySize,
			expectedErrno:   ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=0,)
<== (newoffset=,errno=EFAULT)
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrnoResult(t, tc.expectedErrno, mod, FdSeekName, uint64(tc.fd), tc.offset, uint64(tc.whence), uint64(tc.resultNewoffset))
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
		fd            uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_sync(fd=42)
<== errno=EBADF
`,
		},
		{
			name:          "valid fd",
			fd:            fd,
			expectedErrno: ErrnoSuccess,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdSyncName, uint64(tc.fd))
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
	fsc := mod.(*wasm.CallContext).Sys.FS()
	f, ok := fsc.LookupFile(fd)
	require.True(t, ok)
	seeker := f.File.(io.Seeker)

	// set the initial offset of the file to 1
	offset, err := seeker.Seek(1, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(1), offset)

	requireErrnoResult(t, ErrnoSuccess, mod, FdTellName, uint64(fd), uint64(resultNewoffset))
	require.Equal(t, expectedLog, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	offset, err = seeker.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	require.Equal(t, expectedOffset, offset) // test that the offset of file is actually updated.
}

func Test_fdTell_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, t.TempDir(), "test_path", []byte("wazero"), true)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size()

	tests := []struct {
		name            string
		fd              uint32
		resultNewoffset uint32
		expectedErrno   Errno
		expectedLog     string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_tell(fd=42,result.offset=0)
<== errno=EBADF
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fd,
			resultNewoffset: memorySize,
			expectedErrno:   ErrnoFault,
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

			requireErrnoResult(t, tc.expectedErrno, mod, FdTellName, uint64(tc.fd), uint64(tc.resultNewoffset))
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

	requireErrnoResult(t, ErrnoSuccess, mod, FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2)
<== (nwritten=6,errno=ESUCCESS)
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Since we initialized this file, we know we can read it by path
	buf, err := os.ReadFile(path.Join(tmpDir, pathName))
	require.NoError(t, err)

	require.Equal(t, []byte("wazero"), buf) // verify the file was actually written
}

// Test_fdWrite_discard ensures default configuration doesn't add needless
// overhead, but still returns valid data. For example, writing to STDOUT when
// it is io.Discard.
func Test_fdWrite_discard(t *testing.T) {
	// Default has io.Discard as stdout/stderr
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig())
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

	fd := sys.FdStdout
	requireErrnoResult(t, ErrnoSuccess, mod, FdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	// Should not amplify logging
	require.Zero(t, len(log.Bytes()))

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdWrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenFile(t, tmpDir, pathName, nil, false)
	defer r.Close(testCtx)

	// Setup valid test memory
	iovsCount := uint32(1)
	memSize := mod.Memory().Size()

	tests := []struct {
		name                     string
		fd, iovs, resultNwritten uint32
		expectedErrno            Errno
		expectedLog              string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=42,iovs=0,iovs_len=1)
<== (nwritten=,errno=EBADF)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          memSize - 2,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65534,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "out-of-memory reading iovs[0].length",
			fd:            fd,
			iovs:          memSize - 4, // iovs[0].offset was 4 bytes and iovs[0].length next, but not enough mod.Memory()!
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65532,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "iovs[0].offset is outside memory",
			fd:            fd,
			iovs:          memSize - 5, // iovs[0].offset (where to read "hi") is outside memory.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65531,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:          "length to read exceeds memory by 1",
			fd:            fd,
			iovs:          memSize - 9, // iovs[0].offset (where to read "hi") is in memory, but truncated.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=65527,iovs_len=1)
<== (nwritten=,errno=EFAULT)
`,
		},
		{
			name:           "resultNwritten offset is outside memory",
			fd:             fd,
			resultNwritten: memSize, // read was ok, but there wasn't enough memory to write the result.
			expectedErrno:  ErrnoFault,
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
				leb128.EncodeUint32(tc.iovs+8), // = iovs[0].offset (where the data "hi" begins)
				// = iovs[0].length (how many bytes are in "hi")
				2, 0, 0, 0,
				'h', 'i', // iovs[0].length bytes
			))

			requireErrnoResult(t, tc.expectedErrno, mod, FdWriteName, uint64(tc.fd), uint64(tc.iovs), uint64(iovsCount),
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
	realPath := path.Join(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	preopenedFD := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrnoResult(t, ErrnoSuccess, mod, PathCreateDirectoryName, uint64(preopenedFD), uint64(name), uint64(nameLen))
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
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName    string
		fd, path, pathLen uint32
		expectedErrno     Errno
		expectedLog       string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_create_directory(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoExist,
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
			expectedErrno: ErrnoExist,
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

			requireErrnoResult(t, tc.expectedErrno, mod, PathCreateDirectoryName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
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
		name                        string
		fd, pathLen, resultFilestat uint32
		memory, expectedMemory      []byte
		expectedErrno               Errno
		expectedLog                 string
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
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=-1,flags=,path=)
<== (filestat=,errno=EBADF)
`,
		},
		{
			name:           "FD not a directory",
			fd:             fileFD,
			memory:         initialMemoryFile,
			pathLen:        uint32(len(file)),
			resultFilestat: 2,
			expectedErrno:  ErrnoNotdir,
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
			expectedErrno:  ErrnoNoent,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=,path=animals.txt)
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

			flags := uint32(0)
			requireErrnoResult(t, tc.expectedErrno, mod, PathFilestatGetName, uint64(tc.fd), uint64(flags), uint64(1), uint64(tc.pathLen), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_pathFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_pathFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, PathFilestatSetTimesName, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_filestat_set_times(fd=0,flags=,path=,atim=0,mtim=0,fst_flags=0)
<-- errno=ENOSYS
`, log)
}

func Test_pathLink(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	const oldDirName = "my-old-dir"
	mod, oldFd, log, r := requireOpenFile(t, tmpDir, oldDirName, nil, false)
	defer r.Close(testCtx)

	const newDirName = "my-new-dir/sub"
	require.NoError(t, os.MkdirAll(path.Join(tmpDir, newDirName), 0o700))
	fsc := mod.(*wasm.CallContext).Sys.FS()
	newFd, err := fsc.OpenFile(fsc.RootFS(), newDirName, 0o600, 0)
	require.NoError(t, err)

	mem := mod.Memory()

	const filename = "file"
	err = os.WriteFile(path.Join(tmpDir, oldDirName, filename), []byte{1, 2, 3, 4}, 0o700)
	require.NoError(t, err)

	const fileNamePtr = 0xff
	ok := mem.Write(fileNamePtr, []byte(filename))
	require.True(t, ok)

	const nonExistingFileNamePtr = 0xaa
	const nonExistingFileName = "invalid-san"
	ok = mem.Write(nonExistingFileNamePtr, []byte(nonExistingFileName))
	require.True(t, ok)

	const destinationNamePtr = 0xcc
	const destinationName = "hard-linked"
	ok = mem.Write(destinationNamePtr, []byte(destinationName))
	require.True(t, ok)

	destinationRealPath := path.Join(tmpDir, newDirName, destinationName)

	t.Run("success", func(t *testing.T) {
		requireErrnoResult(t, ErrnoSuccess, mod, PathLinkName,
			uint64(oldFd), 0, fileNamePtr, uint64(len(filename)),
			uint64(newFd), destinationNamePtr, uint64(len(destinationName)))
		require.Contains(t, log.String(), ErrnoName(ErrnoSuccess))

		f, err := os.Open(destinationRealPath)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, f.Close())
		}()

		var st platform.Stat_t
		require.NoError(t, platform.StatFile(f, &st))
		require.NoError(t, err)
		require.False(t, st.Mode&os.ModeSymlink == os.ModeSymlink)
		require.Equal(t, uint64(2), st.Nlink)
	})

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			errno                                              Errno
			oldDirFd, newDirFd, oldPtr, newPtr, oldLen, newLen uint64
		}{
			{errno: ErrnoBadf, oldDirFd: 1000},
			{errno: ErrnoBadf, oldDirFd: uint64(oldFd), newDirFd: 1000},
			{errno: ErrnoNotdir, oldDirFd: uint64(oldFd), newDirFd: 1},
			{errno: ErrnoNotdir, oldDirFd: 1, newDirFd: 1},
			{errno: ErrnoNotdir, oldDirFd: 1, newDirFd: uint64(newFd)},
			{errno: ErrnoFault, oldDirFd: uint64(oldFd), newDirFd: uint64(newFd), oldLen: math.MaxUint32},
			{errno: ErrnoFault, oldDirFd: uint64(oldFd), newDirFd: uint64(newFd), newLen: math.MaxUint32},
			{
				errno: ErrnoFault, oldDirFd: uint64(oldFd), newDirFd: uint64(newFd),
				oldPtr: math.MaxUint32, oldLen: 100, newLen: 100,
			},
			{
				errno: ErrnoFault, oldDirFd: uint64(oldFd), newDirFd: uint64(newFd),
				oldPtr: 1, oldLen: 100, newPtr: math.MaxUint32, newLen: 100,
			},
		} {
			name := ErrnoName(tc.errno)
			t.Run(name, func(t *testing.T) {
				requireErrnoResult(t, tc.errno, mod, PathLinkName,
					tc.oldDirFd, 0, tc.oldPtr, tc.oldLen,
					tc.newDirFd, tc.newPtr, tc.newLen)
				require.Contains(t, log.String(), name)
			})
		}
	})
}

func Test_pathOpen(t *testing.T) {
	dir := t.TempDir() // open before loop to ensure no locking problems.
	writeFS := sysfs.NewDirFS(dir)
	readFS := sysfs.NewReadFS(writeFS)

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

	dirFileName := path.Join(dirName, fileName)
	dirFileContents := []byte("def")
	writeFile(t, dir, dirFileName, dirFileContents)

	expectedOpenedFd := sys.FdPreopen + 1

	tests := []struct {
		name          string
		fs            sysfs.FS
		path          func(t *testing.T) string
		oflags        uint16
		fdflags       uint16
		expected      func(t *testing.T, fsc *sys.FSContext)
		expectedErrno Errno
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
			fdflags:       FD_APPEND,
			path:          func(t *testing.T) (file string) { return appendName },
			expectedErrno: ErrnoNosys,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=append,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=APPEND)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:    "sysfs.DirFS FD_APPEND",
			fs:      writeFS,
			path:    func(t *testing.T) (file string) { return appendName },
			fdflags: FD_APPEND,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

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
			oflags:        O_CREAT,
			expectedErrno: ErrnoNosys,
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
			oflags: O_CREAT,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				// expect to create a new file
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

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
			oflags:        O_CREAT | O_TRUNC,
			expectedErrno: ErrnoNosys,
			path:          func(t *testing.T) (file string) { return path.Join(dirName, "O_CREAT-O_TRUNC") },
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir/O_CREAT-O_TRUNC,oflags=CREAT|TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOSYS)
`,
		},
		{
			name:   "sysfs.DirFS O_CREAT O_TRUNC",
			fs:     writeFS,
			path:   func(t *testing.T) (file string) { return path.Join(dirName, "O_CREAT-O_TRUNC") },
			oflags: O_CREAT | O_TRUNC,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				// expect to create a new file
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

				// verify the contents were written
				b := readFile(t, dir, path.Join(dirName, "O_CREAT-O_TRUNC"))
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
			oflags: O_DIRECTORY,
			path:   func(*testing.T) string { return dirName },
			expected: func(t *testing.T, fsc *sys.FSContext) {
				f, ok := fsc.LookupFile(expectedOpenedFd)
				require.True(t, ok)
				require.True(t, f.IsDir())
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
			oflags: O_DIRECTORY,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				f, ok := fsc.LookupFile(expectedOpenedFd)
				require.True(t, ok)
				require.True(t, f.IsDir())
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
		{
			name:          "sysfs.ReadFS O_TRUNC",
			fs:            readFS,
			oflags:        O_TRUNC,
			expectedErrno: ErrnoNosys,
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
			oflags: O_TRUNC,
			expected: func(t *testing.T, fsc *sys.FSContext) {
				contents := []byte("hello")
				_, err := sys.WriterForFile(fsc, expectedOpenedFd).Write(contents)
				require.NoError(t, err)
				require.NoError(t, fsc.CloseFile(expectedOpenedFd))

				// verify the contents were truncated
				b := readFile(t, dir, "trunc")
				require.Equal(t, contents, b)
			},
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=trunc,oflags=TRUNC,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=4,errno=ESUCCESS)
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
				WithFS(tc.fs.(fs.FS))) // built-in impls reverse-implement fs.FS
			defer r.Close(testCtx)
			pathName := tc.path(t)
			mod.Memory().Write(0, []byte(pathName))

			path := uint32(0)
			pathLen := uint32(len(pathName))
			resultOpenedFd := pathLen
			dirfd := sys.FdPreopen

			// TODO: dirflags is a lookupflags and it only has one bit: symlink_follow
			// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#lookupflags
			dirflags := 0

			// rights aren't used
			fsRightsBase, fsRightsInheriting := uint64(0), uint64(0)

			requireErrnoResult(t, tc.expectedErrno, mod, PathOpenName, uint64(dirfd), uint64(dirflags), uint64(path),
				uint64(pathLen), uint64(tc.oflags), fsRightsBase, fsRightsInheriting, uint64(tc.fdflags), uint64(resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			if tc.expectedErrno == ErrnoSuccess {
				openedFd, ok := mod.Memory().ReadUint32Le(pathLen)
				require.True(t, ok)
				require.Equal(t, expectedOpenedFd, openedFd)

				tc.expected(t, mod.(*wasm.CallContext).Sys.FS())
			}
		})
	}
}

func requireOpenFD(t *testing.T, mod api.Module, path string) uint32 {
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fd, err := fsc.OpenFile(preopen, path, os.O_RDONLY, 0)
	require.NoError(t, err)
	return fd
}

func requireContents(t *testing.T, fsc *sys.FSContext, expectedOpenedFd uint32, fileName string, fileContents []byte) {
	// verify the file was actually opened
	f, ok := fsc.LookupFile(expectedOpenedFd)
	require.True(t, ok)
	require.Equal(t, fileName, f.Name)

	// verify the contents are readable
	b, err := io.ReadAll(f.File)
	require.NoError(t, err)
	require.Equal(t, fileContents, b)
}

func mkdir(t *testing.T, tmpDir, dir string) {
	err := os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)
}

func readFile(t *testing.T, tmpDir, file string) []byte {
	contents, err := os.ReadFile(path.Join(tmpDir, file))
	require.NoError(t, err)
	return contents
}

func writeFile(t *testing.T, tmpDir, file string, contents []byte) {
	err := os.WriteFile(path.Join(tmpDir, file), contents, 0o600)
	require.NoError(t, err)
}

func Test_pathOpen_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	fsConfig := wazero.NewFSConfig().WithDirMount(tmpDir, "/")
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	defer r.Close(testCtx)

	preopenedFD := sys.FdPreopen

	file := "file"
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName                            string
		fd, path, pathLen, oflags, resultOpenedFd uint32
		expectedErrno                             Errno
		expectedLog                               string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=42,dirflags=,path=,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EBADF)
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
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
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=OOM(65536,4),oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            sys.FdPreopen,
			path:          0,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is OOM for path
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=di,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOENT)
`,
		},
		{
			name:           "out-of-memory writing resultOpenedFd",
			fd:             preopenedFD,
			pathName:       dir,
			path:           0,
			pathLen:        uint32(len(dir)),
			resultOpenedFd: mod.Memory().Size(), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=dir,oflags=,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=EFAULT)
`,
		},
		{
			name:          "O_DIRECTORY, but not a directory",
			oflags:        uint32(O_DIRECTORY),
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=,path=file,oflags=DIRECTORY,fs_rights_base=,fs_rights_inheriting=,fdflags=)
<== (opened_fd=,errno=ENOTDIR)
`,
		},
		{
			name:          "oflags=directory and create invalid",
			oflags:        uint32(O_DIRECTORY | O_CREAT),
			fd:            sys.FdPreopen,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoInval,
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

			requireErrnoResult(t, tc.expectedErrno, mod, PathOpenName, uint64(tc.fd), uint64(0), uint64(tc.path),
				uint64(tc.pathLen), uint64(tc.oflags), 0, 0, 0, uint64(tc.resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathReadlink(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	const topDirName = "top"
	mod, topFd, log, r := requireOpenFile(t, tmpDir, topDirName, nil, false)
	defer r.Close(testCtx)

	const subDirName = "sub-dir"
	require.NoError(t, os.Mkdir(path.Join(tmpDir, topDirName, subDirName), 0o700))

	mem := mod.Memory()

	const originalFileName = "top-original-file"
	const destinationPathNamePtr = 0x77
	const destinationPathName = "top-symlinked"
	ok := mem.Write(destinationPathNamePtr, []byte(destinationPathName))
	require.True(t, ok)

	originalSubDirFileName := path.Join(subDirName, "subdir-original-file")
	destinationSubDirFileName := path.Join(subDirName, "subdir-symlinked")
	const destinationSubDirPathNamePtr = 0xcc
	ok = mem.Write(destinationSubDirPathNamePtr, []byte(destinationSubDirFileName))
	require.True(t, ok)

	// Create original file and symlink to the destination.
	originalRelativePath := path.Join(topDirName, originalFileName)
	err := os.WriteFile(path.Join(tmpDir, originalRelativePath), []byte{4, 3, 2, 1}, 0o700)
	require.NoError(t, err)
	err = os.Symlink(originalRelativePath, path.Join(tmpDir, topDirName, destinationPathName))
	require.NoError(t, err)
	originalSubDirRelativePath := path.Join(topDirName, originalSubDirFileName)
	err = os.WriteFile(path.Join(tmpDir, originalSubDirRelativePath), []byte{1, 2, 3, 4}, 0o700)
	require.NoError(t, err)
	err = os.Symlink(originalSubDirRelativePath, path.Join(tmpDir, topDirName, destinationSubDirFileName))
	require.NoError(t, err)

	t.Run("ok", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			pathPtr uint64
			pathLen uint64
			exp     string
		}{
			{
				name:    "top",
				pathPtr: destinationPathNamePtr,
				pathLen: uint64(len(destinationPathName)),
				exp:     originalRelativePath,
			},
			{
				name:    "subdir",
				pathPtr: destinationSubDirPathNamePtr,
				pathLen: uint64(len(destinationSubDirFileName)),
				exp:     originalSubDirRelativePath,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				const bufPtr, bufSize, resultBufUsedPtr = 0x100, 0xff, 0x200
				requireErrnoResult(t, ErrnoSuccess, mod, PathReadlinkName,
					uint64(topFd),
					tc.pathPtr, tc.pathLen,
					bufPtr, bufSize, resultBufUsedPtr)
				require.Contains(t, log.String(), ErrnoName(ErrnoSuccess))

				size, ok := mem.ReadUint32Le(resultBufUsedPtr)
				require.True(t, ok)
				actual, ok := mem.Read(bufPtr, size)
				require.True(t, ok)
				require.Equal(t, tc.exp, string(actual))
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			errno                                                     Errno
			dirFd, pathPtr, pathLen, bufPtr, bufLen, resultBufUsedPtr uint64
		}{
			{errno: ErrnoInval},
			{errno: ErrnoInval, pathLen: 100},
			{errno: ErrnoInval, bufLen: 100},
			{errno: ErrnoFault, dirFd: uint64(topFd), bufLen: 100, pathLen: 100, bufPtr: math.MaxUint32},
			{errno: ErrnoFault, bufLen: 100, pathLen: 100, bufPtr: 50, pathPtr: math.MaxUint32},
			{errno: ErrnoNotdir, bufLen: 100, pathLen: 100, bufPtr: 50, pathPtr: 50, dirFd: 1},
			{errno: ErrnoBadf, bufLen: 100, pathLen: 100, bufPtr: 50, pathPtr: 50, dirFd: 1000},
			{
				errno:  ErrnoNoent,
				bufLen: 100, bufPtr: 50,
				pathPtr: destinationPathNamePtr, pathLen: uint64(len(destinationPathName)) - 1,
				dirFd: uint64(topFd),
			},
		} {
			name := ErrnoName(tc.errno)
			t.Run(name, func(t *testing.T) {
				requireErrnoResult(t, tc.errno, mod, PathReadlinkName,
					tc.dirFd, tc.pathPtr, tc.pathLen, tc.bufPtr,
					tc.bufLen, tc.resultBufUsedPtr)
				require.Contains(t, log.String(), name)
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
	realPath := path.Join(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	// create the directory
	err := os.Mkdir(realPath, 0o700)
	require.NoError(t, err)

	dirFD := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrnoResult(t, ErrnoSuccess, mod, PathRemoveDirectoryName, uint64(dirFD), uint64(name), uint64(nameLen))
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
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dirNotEmpty := "notempty"
	err = os.Mkdir(path.Join(tmpDir, dirNotEmpty), 0o700)
	require.NoError(t, err)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dirNotEmpty, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName    string
		fd, path, pathLen uint32
		expectedErrno     Errno
		expectedLog       string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_remove_directory(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			pathName:      file,
			path:          0,
			pathLen:       uint32(len(file)),
			expectedErrno: ErrnoNotdir,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoNoent,
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
			expectedErrno: ErrnoNotdir,
			expectedLog: fmt.Sprintf(`
==> wasi_snapshot_preview1.path_remove_directory(fd=3,path=file)
<== errno=%s
`, ErrnoName(ErrnoNotdir)),
		},
		{
			name:          "dir not empty",
			fd:            sys.FdPreopen,
			pathName:      dirNotEmpty,
			path:          0,
			pathLen:       uint32(len(dirNotEmpty)),
			expectedErrno: ErrnoNotempty,
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

			requireErrnoResult(t, tc.expectedErrno, mod, PathRemoveDirectoryName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_pathSymlink_errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.

	const dirname = "my-dir"
	mod, fd, log, r := requireOpenFile(t, tmpDir, dirname, nil, false)
	defer r.Close(testCtx)

	mem := mod.Memory()

	const filename = "file"
	err := os.WriteFile(path.Join(tmpDir, dirname, filename), []byte{1, 2, 3, 4}, 0o700)
	require.NoError(t, err)

	const fileNamePtr = 0xff
	ok := mem.Write(fileNamePtr, []byte(filename))
	require.True(t, ok)

	const nonExistingFileNamePtr = 0xaa
	const nonExistingFileName = "invalid-san"
	ok = mem.Write(nonExistingFileNamePtr, []byte(nonExistingFileName))
	require.True(t, ok)

	const destinationNamePtr = 0xcc
	const destinationName = "symlinked"
	ok = mem.Write(destinationNamePtr, []byte(destinationName))
	require.True(t, ok)

	t.Run("success", func(t *testing.T) {
		requireErrnoResult(t, ErrnoSuccess, mod, PathSymlinkName,
			fileNamePtr, uint64(len(filename)), uint64(fd), destinationNamePtr, uint64(len(destinationName)))
		require.Contains(t, log.String(), ErrnoName(ErrnoSuccess))
		st, err := os.Lstat(path.Join(tmpDir, dirname, destinationName))
		require.NoError(t, err)
		require.Equal(t, st.Mode()&os.ModeSymlink, os.ModeSymlink)
	})

	t.Run("errors", func(t *testing.T) {
		for _, tc := range []struct {
			errno                                 Errno
			dirFd, oldPtr, newPtr, oldLen, newLen uint64
		}{
			{errno: ErrnoBadf, dirFd: 1000},
			{errno: ErrnoNotdir, dirFd: 2},
			// Length zero buffer is not valid.
			{errno: ErrnoInval, dirFd: uint64(fd)},
			{errno: ErrnoInval, oldLen: 100, dirFd: uint64(fd)},
			{errno: ErrnoInval, newLen: 100, dirFd: uint64(fd)},
			// Invalid pointer to the names.
			{errno: ErrnoFault, oldPtr: math.MaxUint64, oldLen: 100, newLen: 100, dirFd: uint64(fd)},
			{errno: ErrnoFault, newPtr: math.MaxUint64, oldLen: 100, newLen: 100, dirFd: uint64(fd)},
			{errno: ErrnoFault, oldPtr: math.MaxUint64, newPtr: math.MaxUint64, oldLen: 100, newLen: 100, dirFd: uint64(fd)},
			// Non-existing path as source.
			{
				errno: ErrnoInval, oldPtr: nonExistingFileNamePtr, oldLen: uint64(len(nonExistingFileName)),
				newPtr: 0, newLen: 5, dirFd: uint64(fd),
			},
			// Linking to existing file.
			{
				errno: ErrnoExist, oldPtr: fileNamePtr, oldLen: uint64(len(filename)),
				newPtr: fileNamePtr, newLen: uint64(len(filename)), dirFd: uint64(fd),
			},
		} {
			name := ErrnoName(tc.errno)
			t.Run(name, func(t *testing.T) {
				requireErrnoResult(t, tc.errno, mod, PathSymlinkName,
					tc.oldPtr, tc.oldLen, tc.dirFd, tc.newPtr, tc.newLen)
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
	oldDirFD := sys.FdPreopen
	oldPathName := "wazero"
	realOldPath := path.Join(tmpDir, oldPathName)
	oldPath := uint32(0)
	oldPathLen := len(oldPathName)
	ok := mod.Memory().Write(oldPath, []byte(oldPathName))
	require.True(t, ok)

	// create the file
	err := os.WriteFile(realOldPath, []byte{}, 0o600)
	require.NoError(t, err)

	newDirFD := sys.FdPreopen
	newPathName := "wahzero"
	realNewPath := path.Join(tmpDir, newPathName)
	newPath := uint32(16)
	newPathLen := len(newPathName)
	ok = mod.Memory().Write(newPath, []byte(newPathName))
	require.True(t, ok)

	requireErrnoResult(t, ErrnoSuccess, mod, PathRenameName,
		uint64(oldDirFD), uint64(oldPath), uint64(oldPathLen),
		uint64(newDirFD), uint64(newPath), uint64(newPathLen))
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
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)

	// We have to test FD validation with a path not under test. Otherwise,
	// Windows may fail for the wrong reason, like:
	//	The process cannot access the file because it is being used by another process.
	file1 := "file1"
	err = os.WriteFile(path.Join(tmpDir, file1), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file1)

	dirNotEmpty := "notempty"
	err = os.Mkdir(path.Join(tmpDir, dirNotEmpty), 0o700)
	require.NoError(t, err)

	dir := path.Join(dirNotEmpty, "dir")
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, oldPathName, newPathName string
		oldFd, oldPath, oldPathLen     uint32
		newFd, newPath, newPathLen     uint32
		expectedErrno                  Errno
		expectedLog                    string
	}{
		{
			name:          "unopened old fd",
			oldFd:         42, // arbitrary invalid fd
			newFd:         sys.FdPreopen,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=42,old_path=,new_fd=3,new_path=)
<== errno=EBADF
`,
		},
		{
			name:          "old FD not a directory",
			oldFd:         fileFD,
			newFd:         sys.FdPreopen,
			expectedErrno: ErrnoNotdir,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=4,old_path=,new_fd=3,new_path=)
<== errno=ENOTDIR
`,
		},
		{
			name:          "unopened new fd",
			oldFd:         sys.FdPreopen,
			newFd:         42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_rename(fd=3,old_path=,new_fd=42,new_path=)
<== errno=EBADF
`,
		},
		{
			name:          "new FD not a directory",
			oldFd:         sys.FdPreopen,
			newFd:         fileFD,
			expectedErrno: ErrnoNotdir,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoNoent,
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
			expectedErrno: ErrnoIsdir,
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

			requireErrnoResult(t, tc.expectedErrno, mod, PathRenameName,
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
	realPath := path.Join(tmpDir, pathName)
	ok := mod.Memory().Write(0, append([]byte{'?'}, pathName...))
	require.True(t, ok)

	// create the file
	err := os.WriteFile(realPath, []byte{}, 0o600)
	require.NoError(t, err)

	dirFD := sys.FdPreopen
	name := 1
	nameLen := len(pathName)

	requireErrnoResult(t, ErrnoSuccess, mod, PathUnlinkFileName, uint64(dirFD), uint64(name), uint64(nameLen))
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
	err := os.WriteFile(path.Join(tmpDir, file), []byte{}, 0o700)
	require.NoError(t, err)
	fileFD := requireOpenFD(t, mod, file)

	dir := "dir"
	err = os.Mkdir(path.Join(tmpDir, dir), 0o700)
	require.NoError(t, err)

	tests := []struct {
		name, pathName    string
		fd, path, pathLen uint32
		expectedErrno     Errno
		expectedLog       string
	}{
		{
			name:          "unopened FD",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_unlink_file(fd=42,path=)
<== errno=EBADF
`,
		},
		{
			name:          "FD not a directory",
			fd:            fileFD,
			expectedErrno: ErrnoNotdir,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoFault,
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
			expectedErrno: ErrnoNoent,
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
			expectedErrno: ErrnoIsdir,
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

			requireErrnoResult(t, tc.expectedErrno, mod, PathUnlinkFileName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func requireOpenFile(t *testing.T, tmpDir string, pathName string, data []byte, readOnly bool) (api.Module, uint32, *bytes.Buffer, api.Closer) {
	oflags := os.O_RDWR

	realPath := path.Join(tmpDir, pathName)
	if data == nil {
		oflags = os.O_RDONLY
		require.NoError(t, os.Mkdir(realPath, 0o700))
	} else {
		require.NoError(t, os.WriteFile(realPath, data, 0o600))
	}

	fsConfig := wazero.NewFSConfig()

	if readOnly {
		oflags = os.O_RDONLY
		fsConfig = fsConfig.WithReadOnlyDirMount(tmpDir, "/")
	} else {
		fsConfig = fsConfig.WithDirMount(tmpDir, "/")
	}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFSConfig(fsConfig))
	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	fd, err := fsc.OpenFile(preopen, pathName, oflags, 0)
	require.NoError(t, err)

	return mod, fd, log, r
}

// Test_fdReaddir_opened_file_written ensures that writing files to the already-opened directory
// is visible. This is significant on Windows.
// https://github.com/ziglang/zig/blob/2ccff5115454bab4898bae3de88f5619310bc5c1/lib/std/fs/test.zig#L156-L184
func Test_fdReaddir_opened_file_written(t *testing.T) {
	root := t.TempDir()
	mod, r, _ := requireProxyModule(t, wazero.NewModuleConfig().
		WithFSConfig(wazero.NewFSConfig().WithDirMount(root, "/")),
	)
	defer r.Close(testCtx)

	mem := mod.Memory()

	fsc := mod.(*wasm.CallContext).Sys.FS()
	preopen := fsc.RootFS()

	const readDirTarget = "dir"
	mem.Write(0, []byte(readDirTarget))
	requireErrnoResult(t, ErrnoSuccess, mod, PathCreateDirectoryName,
		uint64(sys.FdPreopen), uint64(0), uint64(len(readDirTarget)))

	// Open the directory, before writing files!
	dirFd, err := fsc.OpenFile(preopen, readDirTarget, os.O_RDONLY, 0)
	require.NoError(t, err)

	// Then write a file to the directory.
	f, err := os.Create(path.Join(root, readDirTarget, "afile"))
	require.NoError(t, err)
	defer f.Close()

	// Try list them!
	resultBufused := uint32(0) // where to write the amount used out of bufLen
	buf := uint32(8)           // where to start the dirents
	requireErrnoResult(t, ErrnoSuccess, mod, FdReaddirName,
		uint64(dirFd), uint64(buf), uint64(0x2000), 0, uint64(resultBufused))

	used, _ := mem.ReadUint32Le(resultBufused)

	results, _ := mem.Read(buf, used)
	require.Equal(t, append(append(direntDot, direntDotDot...),
		3, 0, 0, 0, 0, 0, 0, 0, // d_next = 3
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		5, 0, 0, 0, // d_namlen = 4 character
		4, 0, 0, 0, // d_type = regular_file
		'a', 'f', 'i', 'l', 'e', // name
	), results)
}
