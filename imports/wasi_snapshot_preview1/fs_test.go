package wasi_snapshot_preview1

import (
	"bytes"
	_ "embed"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Test_fdAdvise only tests it is stubbed for GrainLang per #271
func Test_fdAdvise(t *testing.T) {
	log := requireErrnoNosys(t, fdAdviseName, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.fd_advise(fd=0,offset=0,len=0,advice=0)
	--> wasi_snapshot_preview1.fd_advise(fd=0,offset=0,len=0,advice=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_fdAllocate only tests it is stubbed for GrainLang per #271
func Test_fdAllocate(t *testing.T) {
	log := requireErrnoNosys(t, fdAllocateName, 0, 0, 0)
	require.Equal(t, `
--> proxy.fd_allocate(fd=0,offset=0,len=0)
	--> wasi_snapshot_preview1.fd_allocate(fd=0,offset=0,len=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdClose(t *testing.T) {
	// fd_close needs to close an open file descriptor. Open two files so that we can tell which is closed.
	path1, path2 := "a", "b"
	testFS := fstest.MapFS{path1: {Data: make([]byte, 0)}, path2: {Data: make([]byte, 0)}}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fdToClose, err := fsc.OpenFile(path1)
	require.NoError(t, err)

	fdToKeep, err := fsc.OpenFile(path2)
	require.NoError(t, err)

	// Close
	requireErrno(t, ErrnoSuccess, mod, fdCloseName, uint64(fdToClose))
	require.Equal(t, `
--> proxy.fd_close(fd=4)
	==> wasi_snapshot_preview1.fd_close(fd=4)
	<== ESUCCESS
<-- 0
`, "\n"+log.String())

	// Verify fdToClose is closed and removed from the opened FDs.
	_, ok := fsc.OpenedFile(fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.OpenedFile(fdToKeep)
	require.True(t, ok)

	log.Reset()
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		requireErrno(t, ErrnoBadf, mod, fdCloseName, uint64(42)) // 42 is an arbitrary invalid FD
		require.Equal(t, `
--> proxy.fd_close(fd=42)
	==> wasi_snapshot_preview1.fd_close(fd=42)
	<== EBADF
<-- 8
`, "\n"+log.String())
	})
}

// Test_fdDatasync only tests it is stubbed for GrainLang per #271
func Test_fdDatasync(t *testing.T) {
	log := requireErrnoNosys(t, fdDatasyncName, 0)
	require.Equal(t, `
--> proxy.fd_datasync(fd=0)
	--> wasi_snapshot_preview1.fd_datasync(fd=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdFdstatGet(t *testing.T) {
	file, dir := "a", "b"
	testFS := fstest.MapFS{file: {Data: make([]byte, 10), ModTime: time.Unix(1667482413, 0)}, dir: {Mode: fs.ModeDir, ModTime: time.Unix(1667482413, 0)}}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fileFd, err := fsc.OpenFile(file)
	require.NoError(t, err)

	dirFd, err := fsc.OpenFile(dir)
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
			fd:   internalsys.FdStdin,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
--> proxy.fd_fdstat_get(fd=0,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=0,result.stat=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "stdout",
			fd:   internalsys.FdStdout,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				1, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
--> proxy.fd_fdstat_get(fd=1,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=1,result.stat=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "stderr",
			fd:   internalsys.FdStderr,
			expectedMemory: []byte{
				1, 0, // fs_filetype
				1, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
--> proxy.fd_fdstat_get(fd=2,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=2,result.stat=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "root",
			fd:   internalsys.FdRoot,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
--> proxy.fd_fdstat_get(fd=3,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=3,result.stat=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "file",
			fd:   fileFd,
			expectedMemory: []byte{
				4, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
--> proxy.fd_fdstat_get(fd=4,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=4,result.stat=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "dir",
			fd:   dirFd,
			expectedMemory: []byte{
				3, 0, // fs_filetype
				0, 0, 0, 0, 0, 0, // fs_flags
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_base
				0, 0, 0, 0, 0, 0, 0, 0, // fs_rights_inheriting
			},
			expectedLog: `
--> proxy.fd_fdstat_get(fd=5,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=5,result.stat=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name:          "bad FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.fd_fdstat_get(fd=-1,result.stat=0)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=-1,result.stat=0)
	<== EBADF
<-- 8
`,
		},
		{
			name:          "resultFdstat exceeds the maximum valid address by 1",
			fd:            dirFd,
			resultFdstat:  memorySize - 24 + 1,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_fdstat_get(fd=5,result.stat=65513)
	==> wasi_snapshot_preview1.fd_fdstat_get(fd=5,result.stat=65513)
	<== EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrno(t, tc.expectedErrno, mod, fdFdstatGetName, uint64(tc.fd), uint64(tc.resultFdstat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_fdFdstatSetFlags only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetFlags(t *testing.T) {
	log := requireErrnoNosys(t, fdFdstatSetFlagsName, 0, 0)
	require.Equal(t, `
--> proxy.fd_fdstat_set_flags(fd=0,flags=0)
	--> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=0,flags=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_fdFdstatSetRights only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetRights(t *testing.T) {
	log := requireErrnoNosys(t, fdFdstatSetRightsName, 0, 0, 0)
	require.Equal(t, `
--> proxy.fd_fdstat_set_rights(fd=0,fs_rights_base=0,fs_rights_inheriting=0)
	--> wasi_snapshot_preview1.fd_fdstat_set_rights(fd=0,fs_rights_base=0,fs_rights_inheriting=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdFilestatGet(t *testing.T) {
	file, dir := "a", "b"
	testFS := fstest.MapFS{
		".":  {Mode: 0o755 | fs.ModeDir, ModTime: time.Unix(0, 0)},
		file: {Data: make([]byte, 10), ModTime: time.Unix(1667482413, 0)},
		dir:  {Mode: fs.ModeDir, ModTime: time.Unix(1667482413, 0)},
	}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	fileFd, err := fsc.OpenFile(file)
	require.NoError(t, err)

	dirFd, err := fsc.OpenFile(dir)
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
			fd:   internalsys.FdStdin,
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
--> proxy.fd_filestat_get(fd=0,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=0,result.buf=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "stdout",
			fd:   internalsys.FdStdout,
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
--> proxy.fd_filestat_get(fd=1,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=1,result.buf=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "stderr",
			fd:   internalsys.FdStderr,
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
--> proxy.fd_filestat_get(fd=2,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=2,result.buf=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "root",
			fd:   internalsys.FdRoot,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // TODO: size
				0, 0, 0, 0, 0, 0, 0, 0, // TODO: atim
				0, 0, 0, 0, 0, 0, 0, 0, // TODO: mtim
				0, 0, 0, 0, 0, 0, 0, 0, // TODO: ctim
			},
			expectedLog: `
--> proxy.fd_filestat_get(fd=3,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=3,result.buf=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "file",
			fd:   fileFd,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				10, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			},
			expectedLog: `
--> proxy.fd_filestat_get(fd=4,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=4,result.buf=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name: "dir",
			fd:   dirFd,
			expectedMemory: []byte{
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			},
			expectedLog: `
--> proxy.fd_filestat_get(fd=5,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=5,result.buf=0)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name:          "bad FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.fd_filestat_get(fd=-1,result.buf=0)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=-1,result.buf=0)
	<== EBADF
<-- 8
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             dirFd,
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.fd_filestat_get(fd=5,result.buf=65473)
	==> wasi_snapshot_preview1.fd_filestat_get(fd=5,result.buf=65473)
	<== EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, mod, len(tc.expectedMemory))

			requireErrno(t, tc.expectedErrno, mod, fdFilestatGetName, uint64(tc.fd), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_fdFilestatSetSize only tests it is stubbed for GrainLang per #271
func Test_fdFilestatSetSize(t *testing.T) {
	log := requireErrnoNosys(t, fdFilestatSetSizeName, 0, 0)
	require.Equal(t, `
--> proxy.fd_filestat_set_size(fd=0,size=0)
	--> wasi_snapshot_preview1.fd_filestat_set_size(fd=0,size=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_fdFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_fdFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, fdFilestatSetTimesName, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.fd_filestat_set_times(fd=0,atim=0,mtim=0,fst_flags=0)
	--> wasi_snapshot_preview1.fd_filestat_set_times(fd=0,atim=0,mtim=0,fst_flags=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdPread(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, "/test_path", []byte("wazero"))
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
--> proxy.fd_pread(fd=4,iovs=1,iovs_len=2,offset=0,result.nread=26)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=0,result.nread=26)
		==> wasi_snapshot_preview1.fdPread(fd=4,iovs=1,iovs_len=2,offset=0)
		<== (nread=6,ESUCCESS)
	<-- ESUCCESS
<-- 0
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
--> proxy.fd_pread(fd=4,iovs=1,iovs_len=2,offset=2,result.nread=26)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=1,iovs_len=2,offset=2,result.nread=26)
		==> wasi_snapshot_preview1.fdPread(fd=4,iovs=1,iovs_len=2,offset=2)
		<== (nread=4,ESUCCESS)
	<-- ESUCCESS
<-- 0
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

			requireErrno(t, ErrnoSuccess, mod, fdPreadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(tc.offset), uint64(resultNread))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

func Test_fdPread_Errors(t *testing.T) {
	contents := []byte("wazero")
	mod, fd, log, r := requireOpenFile(t, "/test_path", contents)
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
--> proxy.fd_pread(fd=42,iovs=65532,iovs_len=65532,offset=0,result.nread=65532)
	--> wasi_snapshot_preview1.fd_pread(fd=42,iovs=65532,iovs_len=65532,offset=0,result.nread=65532)
		==> wasi_snapshot_preview1.fdPread(fd=42,iovs=65532,iovs_len=65532,offset=0)
		<== (nread=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "seek past file",
			fd:            fd,
			offset:        int64(len(contents) + 1),
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_pread(fd=4,iovs=65536,iovs_len=65536,offset=7,result.nread=65536)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65536,iovs_len=65536,offset=7,result.nread=65536)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_pread(fd=4,iovs=65536,iovs_len=65535,offset=0,result.nread=65535)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65536,iovs_len=65535,offset=0,result.nread=65535)
	<-- EFAULT
<-- 21
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
--> proxy.fd_pread(fd=4,iovs=65532,iovs_len=65532,offset=0,result.nread=65531)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65532,iovs_len=65532,offset=0,result.nread=65531)
		==> wasi_snapshot_preview1.fdPread(fd=4,iovs=65532,iovs_len=65532,offset=0)
		<== (nread=0,EFAULT)
	<-- EFAULT
<-- 21
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
--> proxy.fd_pread(fd=4,iovs=65528,iovs_len=65528,offset=0,result.nread=65527)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65528,iovs_len=65528,offset=0,result.nread=65527)
		==> wasi_snapshot_preview1.fdPread(fd=4,iovs=65528,iovs_len=65528,offset=0)
		<== (nread=0,EFAULT)
	<-- EFAULT
<-- 21
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
--> proxy.fd_pread(fd=4,iovs=65527,iovs_len=65527,offset=0,result.nread=65526)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65527,iovs_len=65527,offset=0,result.nread=65526)
		==> wasi_snapshot_preview1.fdPread(fd=4,iovs=65527,iovs_len=65527,offset=0)
		<== (nread=0,EFAULT)
	<-- EFAULT
<-- 21
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
--> proxy.fd_pread(fd=4,iovs=65527,iovs_len=65527,offset=0,result.nread=65536)
	--> wasi_snapshot_preview1.fd_pread(fd=4,iovs=65527,iovs_len=65527,offset=0,result.nread=65536)
	<-- EFAULT
<-- 21
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

			requireErrno(t, tc.expectedErrno, mod, fdPreadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.MapFS{}))
	defer r.Close(testCtx)
	fd := internalsys.FdRoot // only pre-opened directory currently supported.

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

	requireErrno(t, ErrnoSuccess, mod, fdPrestatGetName, uint64(fd), uint64(resultPrestat))
	require.Equal(t, `
--> proxy.fd_prestat_get(fd=3,result.prestat=1)
	--> wasi_snapshot_preview1.fd_prestat_get(fd=3,result.prestat=1)
		==> wasi_snapshot_preview1.fdPrestatGet(fd=3)
		<== (prestat=4294967296,ESUCCESS)
	<-- ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatGet_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.MapFS{}))
	defer r.Close(testCtx)
	fd := internalsys.FdRoot // only pre-opened directory currently supported.

	memorySize := mod.Memory().Size()
	tests := []struct {
		name          string
		fd            uint32
		resultPrestat uint32
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name:          "invalid FD",
			fd:            42, // arbitrary invalid FD
			resultPrestat: 0,  // valid offset
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.fd_prestat_get(fd=42,result.prestat=0)
	--> wasi_snapshot_preview1.fd_prestat_get(fd=42,result.prestat=0)
		==> wasi_snapshot_preview1.fdPrestatGet(fd=42)
		<== (prestat=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "out-of-memory resultPrestat",
			fd:            fd,
			resultPrestat: memorySize,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_prestat_get(fd=3,result.prestat=65536)
	--> wasi_snapshot_preview1.fd_prestat_get(fd=3,result.prestat=65536)
	<-- EFAULT
<-- 21
`,
		},
		// TODO: non pre-opened file == api.ErrnoBadf
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, fdPrestatGetName, uint64(tc.fd), uint64(tc.resultPrestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatDirName(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.MapFS{}))
	defer r.Close(testCtx)
	fd := internalsys.FdRoot // only pre-opened directory currently supported.

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(0) // shorter than len("/") to prove truncation is ok
	expectedMemory := []byte{
		'?', '?', '?', '?',
	}

	maskMemory(t, mod, len(expectedMemory))

	requireErrno(t, ErrnoSuccess, mod, fdPrestatDirNameName, uint64(fd), uint64(path), uint64(pathLen))
	require.Equal(t, `
--> proxy.fd_prestat_dir_name(fd=3,path=1,path_len=0)
	==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3,path=1,path_len=0)
	<== ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatDirName_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fstest.MapFS{}))
	defer r.Close(testCtx)
	fd := internalsys.FdRoot // only pre-opened directory currently supported.

	memorySize := mod.Memory().Size()
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
			fd:            fd,
			path:          memorySize,
			pathLen:       pathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_prestat_dir_name(fd=3,path=65536,path_len=1)
	==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3,path=65536,path_len=1)
	<== EFAULT
<-- 21
`,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            fd,
			path:          memorySize - pathLen + 1,
			pathLen:       pathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_prestat_dir_name(fd=3,path=65536,path_len=1)
	==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3,path=65536,path_len=1)
	<== EFAULT
<-- 21
`,
		},
		{
			name:          "pathLen exceeds the length of the dir name",
			fd:            fd,
			path:          validAddress,
			pathLen:       pathLen + 1,
			expectedErrno: ErrnoNametoolong,
			expectedLog: `
--> proxy.fd_prestat_dir_name(fd=3,path=0,path_len=2)
	==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=3,path=0,path_len=2)
	<== ENAMETOOLONG
<-- 37
`,
		},
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.fd_prestat_dir_name(fd=42,path=0,path_len=1)
	==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=42,path=0,path_len=1)
	<== EBADF
<-- 8
`,
		},
		// TODO: non pre-opened file == ErrnoBadf
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, fdPrestatDirNameName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdPwrite only tests it is stubbed for GrainLang per #271
func Test_fdPwrite(t *testing.T) {
	log := requireErrnoNosys(t, fdPwriteName, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.fd_pwrite(fd=0,iovs=0,iovs_len=0,offset=0,result.nwritten=0)
	--> wasi_snapshot_preview1.fd_pwrite(fd=0,iovs=0,iovs_len=0,offset=0,result.nwritten=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdRead(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, "/test_path", []byte("wazero"))
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

	requireErrno(t, ErrnoSuccess, mod, fdReadName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNread))
	require.Equal(t, `
--> proxy.fd_read(fd=4,iovs=1,iovs_len=2,result.nread=26)
	--> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=2,result.nread=26)
		==> wasi_snapshot_preview1.fdRead(fd=4,iovs=1,iovs_len=2)
		<== (nread=6,ESUCCESS)
	<-- ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdRead_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, "/test_path", []byte("wazero"))
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
--> proxy.fd_read(fd=42,iovs=65532,iovs_len=65532,result.nread=65532)
	--> wasi_snapshot_preview1.fd_read(fd=42,iovs=65532,iovs_len=65532,result.nread=65532)
		==> wasi_snapshot_preview1.fdRead(fd=42,iovs=65532,iovs_len=65532)
		<== (nread=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_read(fd=4,iovs=65536,iovs_len=65535,result.nread=65535)
	--> wasi_snapshot_preview1.fd_read(fd=4,iovs=65536,iovs_len=65535,result.nread=65535)
	<-- EFAULT
<-- 21
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
--> proxy.fd_read(fd=4,iovs=65532,iovs_len=65532,result.nread=65531)
	--> wasi_snapshot_preview1.fd_read(fd=4,iovs=65532,iovs_len=65532,result.nread=65531)
		==> wasi_snapshot_preview1.fdRead(fd=4,iovs=65532,iovs_len=65532)
		<== (nread=0,EFAULT)
	<-- EFAULT
<-- 21
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
--> proxy.fd_read(fd=4,iovs=65528,iovs_len=65528,result.nread=65527)
	--> wasi_snapshot_preview1.fd_read(fd=4,iovs=65528,iovs_len=65528,result.nread=65527)
		==> wasi_snapshot_preview1.fdRead(fd=4,iovs=65528,iovs_len=65528)
		<== (nread=0,EFAULT)
	<-- EFAULT
<-- 21
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
--> proxy.fd_read(fd=4,iovs=65527,iovs_len=65527,result.nread=65526)
	--> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527,result.nread=65526)
		==> wasi_snapshot_preview1.fdRead(fd=4,iovs=65527,iovs_len=65527)
		<== (nread=0,EFAULT)
	<-- EFAULT
<-- 21
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
--> proxy.fd_read(fd=4,iovs=65527,iovs_len=65527,result.nread=65536)
	--> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527,result.nread=65536)
	<-- EFAULT
<-- 21
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

			requireErrno(t, tc.expectedErrno, mod, fdReadName, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.resultNread+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdRead_shouldContinueRead(t *testing.T) {
	tests := []struct {
		name          string
		n, l          uint32
		err           error
		expectedOk    bool
		expectedErrno Errno
	}{
		{
			name: "break when nothing to read",
			n:    0,
			l:    0,
		},
		{
			name: "break when nothing read",
			n:    0,
			l:    4,
		},
		{
			name: "break on partial read",
			n:    3,
			l:    4,
		},
		{
			name:       "continue on full read",
			n:          4,
			l:          4,
			expectedOk: true,
		},
		{
			name: "break on EOF on nothing to read",
			err:  io.EOF,
		},
		{
			name: "break on EOF on nothing read",
			l:    4,
			err:  io.EOF,
		},
		{
			name: "break on EOF on partial read",
			n:    3,
			l:    4,
			err:  io.EOF,
		},
		{
			name: "break on EOF on full read",
			n:    4,
			l:    4,
			err:  io.EOF,
		},
		{
			name:          "return ErrnoIo on error on nothing to read",
			err:           io.ErrClosedPipe,
			expectedErrno: ErrnoIo,
		},
		{
			name:          "return ErrnoIo on error on nothing read",
			l:             4,
			err:           io.ErrClosedPipe,
			expectedErrno: ErrnoIo,
		},
		{ // Special case, allows processing data before err
			name: "break on error on partial read",
			n:    3,
			l:    4,
			err:  io.ErrClosedPipe,
		},
		{ // Special case, allows processing data before err
			name: "break on error on full read",
			n:    4,
			l:    4,
			err:  io.ErrClosedPipe,
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			ok, errno := fdRead_shouldContinueRead(tc.n, tc.l, tc.err)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expectedErrno, errno)
		})
	}
}

var (
	fdReadDirFs = fstest.MapFS{
		"notdir":   {},
		"emptydir": {Mode: fs.ModeDir},
		"dir":      {Mode: fs.ModeDir},
		"dir/-":    {},                 // len = 24+1 = 25
		"dir/a-":   {Mode: fs.ModeDir}, // len = 24+2 = 26
		"dir/ab-":  {},                 // len = 24+3 = 27
	}

	testDirEntries = func() []fs.DirEntry {
		entries, err := fdReadDirFs.ReadDir("dir")
		if err != nil {
			panic(err)
		}
		return entries
	}()

	dirent1 = []byte{
		1, 0, 0, 0, 0, 0, 0, 0, // d_next = 1
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		1, 0, 0, 0, // d_namlen = 1 character
		4, 0, 0, 0, // d_type = regular_file
		'-', // name
	}
	dirent2 = []byte{
		2, 0, 0, 0, 0, 0, 0, 0, // d_next = 2
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		2, 0, 0, 0, // d_namlen = 1 character
		3, 0, 0, 0, // d_type =  directory
		'a', '-', // name
	}
	dirent3 = []byte{
		3, 0, 0, 0, 0, 0, 0, 0, // d_next = 3
		0, 0, 0, 0, 0, 0, 0, 0, // d_ino = 0
		3, 0, 0, 0, // d_namlen = 3 characters
		4, 0, 0, 0, // d_type = regular_file
		'a', 'b', '-', // name
	}
)

func Test_fdReaddir(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fdReadDirFs))
	defer r.Close(testCtx)

	fsc := mod.(*wasm.CallContext).Sys.FS()

	fd, err := fsc.OpenFile("dir")
	require.NoError(t, err)

	tests := []struct {
		name            string
		dir             func() *internalsys.FileEntry
		bufLen          uint32
		cookie          int64
		expectedMem     []byte
		expectedMemSize int
		expectedBufused uint32
		expectedReadDir *internalsys.ReadDir
	}{
		{
			name: "empty dir",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("emptydir")
				require.NoError(t, err)

				return &internalsys.FileEntry{File: dir}
			},
			bufLen:          direntSize,
			cookie:          0,
			expectedBufused: 0,
			expectedMem:     []byte{},
			expectedReadDir: &internalsys.ReadDir{},
		},
		{
			name: "full read",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)

				return &internalsys.FileEntry{File: dir}
			},
			bufLen:          4096,
			cookie:          0,
			expectedBufused: 78, // length of all entries
			expectedMem:     append(append(dirent1, dirent2...), dirent3...),
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
		},
		{
			name: "can't read name",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)

				return &internalsys.FileEntry{File: dir}
			},
			bufLen:          24, // length is long enough for first, but not the name.
			cookie:          0,
			expectedBufused: 24,           // == bufLen which is the size of the dirent
			expectedMem:     dirent1[:24], // header without name
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
		},
		{
			name: "read exactly first",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)

				return &internalsys.FileEntry{File: dir}
			},
			bufLen:          25, // length is long enough for first + the name, but not more.
			cookie:          0,
			expectedBufused: 25, // length to read exactly first.
			expectedMem:     dirent1,
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
		},
		{
			name: "read exactly second",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &internalsys.FileEntry{
					File: dir,
					ReadDir: &internalsys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          26, // length is long enough for exactly second.
			cookie:          1,  // d_next of first
			expectedBufused: 26, // length to read exactly second.
			expectedMem:     dirent2,
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read second and a little more",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &internalsys.FileEntry{
					File: dir,
					ReadDir: &internalsys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          30, // length is longer than the second entry, but not long enough for a header.
			cookie:          1,  // d_next of first
			expectedBufused: 30, // length to read some more, but not enough for a header, so buf was exhausted.
			expectedMem:     dirent2,
			expectedMemSize: len(dirent2), // we do not want to compare the full buffer since we don't know what the leftover 4 bytes will contain.
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read second and header of third",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &internalsys.FileEntry{
					File: dir,
					ReadDir: &internalsys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          50, // length is longer than the second entry + enough for the header of third.
			cookie:          1,  // d_next of first
			expectedBufused: 50, // length to read exactly second and the header of third.
			expectedMem:     append(dirent2, dirent3[0:24]...),
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read second and third",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				entry, err := dir.(fs.ReadDirFile).ReadDir(1)
				require.NoError(t, err)

				return &internalsys.FileEntry{
					File: dir,
					ReadDir: &internalsys.ReadDir{
						CountRead: 1,
						Entries:   entry,
					},
				}
			},
			bufLen:          53, // length is long enough for second and third.
			cookie:          1,  // d_next of first
			expectedBufused: 53, // length to read exactly one second and third.
			expectedMem:     append(dirent2, dirent3...),
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[1:],
			},
		},
		{
			name: "read exactly third",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				two, err := dir.(fs.ReadDirFile).ReadDir(2)
				require.NoError(t, err)

				return &internalsys.FileEntry{
					File: dir,
					ReadDir: &internalsys.ReadDir{
						CountRead: 2,
						Entries:   two[1:],
					},
				}
			},
			bufLen:          27, // length is long enough for exactly third.
			cookie:          2,  // d_next of second.
			expectedBufused: 27, // length to read exactly third.
			expectedMem:     dirent3,
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[2:],
			},
		},
		{
			name: "read third and beyond",
			dir: func() *internalsys.FileEntry {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				two, err := dir.(fs.ReadDirFile).ReadDir(2)
				require.NoError(t, err)

				return &internalsys.FileEntry{
					File: dir,
					ReadDir: &internalsys.ReadDir{
						CountRead: 2,
						Entries:   two[1:],
					},
				}
			},
			bufLen:          100, // length is long enough for third and more, but there is nothing more.
			cookie:          2,   // d_next of second.
			expectedBufused: 27,  // length to read exactly third.
			expectedMem:     dirent3,
			expectedReadDir: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries[2:],
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Assign the state we are testing
			file, ok := fsc.OpenedFile(fd)
			require.True(t, ok)
			dir := tc.dir()
			defer dir.File.Close()

			file.File = dir.File
			file.ReadDir = dir.ReadDir

			maskMemory(t, mod, int(tc.bufLen))

			resultBufused := uint32(0) // where to write the amount used out of bufLen
			buf := uint32(8)           // where to start the dirents
			requireErrno(t, ErrnoSuccess, mod, fdReaddirName,
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

func Test_fdReaddir_Errors(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(fdReadDirFs))
	defer r.Close(testCtx)
	memLen := mod.Memory().Size()

	fsc := mod.(*wasm.CallContext).Sys.FS()

	dirFD, err := fsc.OpenFile("dir")
	require.NoError(t, err)

	fileFD, err := fsc.OpenFile("notdir")
	require.NoError(t, err)

	tests := []struct {
		name                           string
		dir                            func() *internalsys.FileEntry
		fd, buf, bufLen, resultBufused uint32
		cookie                         int64
		readDir                        *internalsys.ReadDir
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
--> proxy.fd_readdir(fd=4,buf=65536,buf_len=1000,cookie=0,result.bufused=0)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=65536,buf_len=1000,cookie=0,result.bufused=0)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=65536,buf_len=1000,cookie=0)
		<== (bufused=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name: "invalid fd",
			fd:   42,                    // arbitrary invalid fd
			buf:  0, bufLen: direntSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.fd_readdir(fd=42,buf=0,buf_len=24,cookie=0,result.bufused=1000)
	--> wasi_snapshot_preview1.fd_readdir(fd=42,buf=0,buf_len=24,cookie=0,result.bufused=1000)
		==> wasi_snapshot_preview1.fdReaddir(fd=42,buf=0,buf_len=24,cookie=0)
		<== (bufused=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name: "not a dir",
			fd:   fileFD,
			buf:  0, bufLen: direntSize, // enough to read the dirent
			resultBufused: 1000, // arbitrary
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.fd_readdir(fd=5,buf=0,buf_len=24,cookie=0,result.bufused=1000)
	--> wasi_snapshot_preview1.fd_readdir(fd=5,buf=0,buf_len=24,cookie=0,result.bufused=1000)
		==> wasi_snapshot_preview1.fdReaddir(fd=5,buf=0,buf_len=24,cookie=0)
		<== (bufused=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "out-of-memory reading buf",
			fd:            dirFD,
			buf:           memLen,
			bufLen:        1000,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_readdir(fd=4,buf=65536,buf_len=1000,cookie=0,result.bufused=0)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=65536,buf_len=1000,cookie=0,result.bufused=0)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=65536,buf_len=1000,cookie=0)
		<== (bufused=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "out-of-memory reading bufLen",
			fd:            dirFD,
			buf:           memLen - 1,
			bufLen:        1000,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_readdir(fd=4,buf=65535,buf_len=1000,cookie=0,result.bufused=0)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=65535,buf_len=1000,cookie=0,result.bufused=0)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=65535,buf_len=1000,cookie=0)
		<== (bufused=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name: "bufLen must be enough to write a struct",
			fd:   dirFD,
			buf:  0, bufLen: 1,
			resultBufused: 1000,
			expectedErrno: ErrnoInval,
			expectedLog: `
--> proxy.fd_readdir(fd=4,buf=0,buf_len=1,cookie=0,result.bufused=1000)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=1,cookie=0,result.bufused=1000)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=0,buf_len=1,cookie=0)
		<== (bufused=0,EINVAL)
	<-- EINVAL
<-- 28
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
--> proxy.fd_readdir(fd=4,buf=0,buf_len=1000,cookie=1,result.bufused=2000)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=1000,cookie=1,result.bufused=2000)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=0,buf_len=1000,cookie=1)
		<== (bufused=0,EINVAL)
	<-- EINVAL
<-- 28
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
--> proxy.fd_readdir(fd=4,buf=0,buf_len=1000,cookie=1,result.bufused=2000)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=1000,cookie=1,result.bufused=2000)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=0,buf_len=1000,cookie=1)
		<== (bufused=0,EINVAL)
	<-- EINVAL
<-- 28
`,
		},
		{
			name: "negative cookie invalid",
			fd:   dirFD,
			buf:  0, bufLen: 1000,
			cookie:        -1,
			readDir:       &internalsys.ReadDir{CountRead: 1},
			resultBufused: 2000,
			expectedErrno: ErrnoInval,
			expectedLog: `
--> proxy.fd_readdir(fd=4,buf=0,buf_len=1000,cookie=-1,result.bufused=2000)
	--> wasi_snapshot_preview1.fd_readdir(fd=4,buf=0,buf_len=1000,cookie=-1,result.bufused=2000)
		==> wasi_snapshot_preview1.fdReaddir(fd=4,buf=0,buf_len=1000,cookie=-1)
		<== (bufused=0,EINVAL)
	<-- EINVAL
<-- 28
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			// Reset the directory so that tests don't taint each other.
			if file, ok := fsc.OpenedFile(tc.fd); ok && tc.fd == dirFD {
				dir, err := fdReadDirFs.Open("dir")
				require.NoError(t, err)
				defer dir.Close()

				file.File = dir
				file.ReadDir = nil
			}

			requireErrno(t, tc.expectedErrno, mod, fdReaddirName,
				uint64(tc.fd), uint64(tc.buf), uint64(tc.bufLen), uint64(tc.cookie), uint64(tc.resultBufused))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_lastDirEntries(t *testing.T) {
	tests := []struct {
		name            string
		f               *internalsys.ReadDir
		cookie          int64
		expectedEntries []fs.DirEntry
		expectedErrno   Errno
	}{
		{
			name: "no prior call",
		},
		{
			name:          "no prior call, but passed a cookie",
			cookie:        1,
			expectedErrno: ErrnoInval,
		},
		{
			name: "cookie is negative",
			f: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
			cookie:        -1,
			expectedErrno: ErrnoInval,
		},
		{
			name: "cookie is greater than last d_next",
			f: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
			cookie:        5,
			expectedErrno: ErrnoInval,
		},
		{
			name: "cookie is last pos",
			f: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
			cookie:          3,
			expectedEntries: nil,
		},
		{
			name: "cookie is one before last pos",
			f: &internalsys.ReadDir{
				CountRead: 3,
				Entries:   testDirEntries,
			},
			cookie:          2,
			expectedEntries: testDirEntries[2:],
		},
		{
			name: "cookie is before current entries",
			f: &internalsys.ReadDir{
				CountRead: 5,
				Entries:   testDirEntries,
			},
			cookie:        1,
			expectedErrno: ErrnoNosys, // not implemented
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			f := tc.f
			if f == nil {
				f = &internalsys.ReadDir{}
			}
			entries, errno := lastDirEntries(f, tc.cookie)
			require.Equal(t, tc.expectedErrno, errno)
			require.Equal(t, tc.expectedEntries, entries)
		})
	}
}

func Test_maxDirents(t *testing.T) {
	tests := []struct {
		name                        string
		entries                     []fs.DirEntry
		maxLen                      uint32
		expectedCount               uint32
		expectedwriteTruncatedEntry bool
		expectedBufused             uint32
	}{
		{
			name: "no entries",
		},
		{
			name:                        "can't fit one",
			entries:                     testDirEntries,
			maxLen:                      23,
			expectedBufused:             23,
			expectedwriteTruncatedEntry: false,
		},
		{
			name:                        "only fits header",
			entries:                     testDirEntries,
			maxLen:                      24,
			expectedBufused:             24,
			expectedwriteTruncatedEntry: true,
		},
		{
			name:            "one",
			entries:         testDirEntries,
			maxLen:          25,
			expectedCount:   1,
			expectedBufused: 25,
		},
		{
			name:                        "one but not room for two's name",
			entries:                     testDirEntries,
			maxLen:                      25 + 25,
			expectedCount:               1,
			expectedwriteTruncatedEntry: true, // can write direntSize
			expectedBufused:             25 + 25,
		},
		{
			name:            "two",
			entries:         testDirEntries,
			maxLen:          25 + 26,
			expectedCount:   2,
			expectedBufused: 25 + 26,
		},
		{
			name:                        "two but not three's dirent",
			entries:                     testDirEntries,
			maxLen:                      25 + 26 + 20,
			expectedCount:               2,
			expectedwriteTruncatedEntry: false, // 20 + 4 == direntSize
			expectedBufused:             25 + 26 + 20,
		},
		{
			name:                        "two but not three's name",
			entries:                     testDirEntries,
			maxLen:                      25 + 26 + 26,
			expectedCount:               2,
			expectedwriteTruncatedEntry: true, // can write direntSize
			expectedBufused:             25 + 26 + 26,
		},
		{
			name:                        "three",
			entries:                     testDirEntries,
			maxLen:                      25 + 26 + 27,
			expectedCount:               3,
			expectedwriteTruncatedEntry: false, // end of dir
			expectedBufused:             25 + 26 + 27,
		},
		{
			name:                        "max",
			entries:                     testDirEntries,
			maxLen:                      100,
			expectedCount:               3,
			expectedwriteTruncatedEntry: false, // end of dir
			expectedBufused:             25 + 26 + 27,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			bufused, direntCount, writeTruncatedEntry := maxDirents(tc.entries, tc.maxLen)
			require.Equal(t, tc.expectedCount, direntCount)
			require.Equal(t, tc.expectedwriteTruncatedEntry, writeTruncatedEntry)
			require.Equal(t, tc.expectedBufused, bufused)
		})
	}
}

func Test_writeDirents(t *testing.T) {
	tests := []struct {
		name                string
		entries             []fs.DirEntry
		entryCount          uint32
		writeTruncatedEntry bool
		expectedEntriesBuf  []byte
	}{
		{
			name:    "none",
			entries: testDirEntries,
		},
		{
			name:               "one",
			entries:            testDirEntries,
			entryCount:         1,
			expectedEntriesBuf: dirent1,
		},
		{
			name:               "two",
			entries:            testDirEntries,
			entryCount:         2,
			expectedEntriesBuf: append(dirent1, dirent2...),
		},
		{
			name:                "two with truncated",
			entries:             testDirEntries,
			entryCount:          2,
			writeTruncatedEntry: true,
			expectedEntriesBuf:  append(append(dirent1, dirent2...), dirent3[0:10]...),
		},
		{
			name:               "three",
			entries:            testDirEntries,
			entryCount:         3,
			expectedEntriesBuf: append(append(dirent1, dirent2...), dirent3...),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			cookie := uint64(1)
			entriesBuf := make([]byte, len(tc.expectedEntriesBuf))
			writeDirents(tc.entries, tc.entryCount, tc.writeTruncatedEntry, entriesBuf, cookie)
			require.Equal(t, tc.expectedEntriesBuf, entriesBuf)
		})
	}
}

// Test_fdRenumber only tests it is stubbed for GrainLang per #271
func Test_fdRenumber(t *testing.T) {
	log := requireErrnoNosys(t, fdRenumberName, 0, 0)
	require.Equal(t, `
--> proxy.fd_renumber(fd=0,to=0)
	--> wasi_snapshot_preview1.fd_renumber(fd=0,to=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdSeek(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, "/test_path", []byte("wazero"))
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
--> proxy.fd_seek(fd=4,offset=4,whence=0,result.newoffset=1)
	--> wasi_snapshot_preview1.fd_seek(fd=4,offset=4,whence=0,result.newoffset=1)
		==> wasi_snapshot_preview1.fdSeek(fd=4,offset=4,whence=0)
		<== (newoffset=4,ESUCCESS)
	<-- ESUCCESS
<-- 0
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
--> proxy.fd_seek(fd=4,offset=1,whence=1,result.newoffset=1)
	--> wasi_snapshot_preview1.fd_seek(fd=4,offset=1,whence=1,result.newoffset=1)
		==> wasi_snapshot_preview1.fdSeek(fd=4,offset=1,whence=1)
		<== (newoffset=2,ESUCCESS)
	<-- ESUCCESS
<-- 0
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
--> proxy.fd_seek(fd=4,offset=-1,whence=2,result.newoffset=1)
	--> wasi_snapshot_preview1.fd_seek(fd=4,offset=-1,whence=2,result.newoffset=1)
		==> wasi_snapshot_preview1.fdSeek(fd=4,offset=-1,whence=2)
		<== (newoffset=5,ESUCCESS)
	<-- ESUCCESS
<-- 0
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
			f, ok := fsc.OpenedFile(fd)
			require.True(t, ok)
			seeker := f.File.(io.Seeker)

			// set the initial offset of the file to 1
			offset, err := seeker.Seek(1, io.SeekStart)
			require.NoError(t, err)
			require.Equal(t, int64(1), offset)

			requireErrno(t, ErrnoSuccess, mod, fdSeekName, uint64(fd), uint64(tc.offset), uint64(tc.whence), uint64(resultNewoffset))
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
	mod, fd, log, r := requireOpenFile(t, "/test_path", []byte("wazero"))
	defer r.Close(testCtx)

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
--> proxy.fd_seek(fd=42,offset=0,whence=0,result.newoffset=0)
	--> wasi_snapshot_preview1.fd_seek(fd=42,offset=0,whence=0,result.newoffset=0)
		==> wasi_snapshot_preview1.fdSeek(fd=42,offset=0,whence=0)
		<== (newoffset=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "invalid whence",
			fd:            fd,
			whence:        3, // invalid whence, the largest whence io.SeekEnd(2) + 1
			expectedErrno: ErrnoInval,
			expectedLog: `
--> proxy.fd_seek(fd=4,offset=0,whence=3,result.newoffset=0)
	--> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=3,result.newoffset=0)
		==> wasi_snapshot_preview1.fdSeek(fd=4,offset=0,whence=3)
		<== (newoffset=0,EINVAL)
	<-- EINVAL
<-- 28
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fd,
			resultNewoffset: memorySize,
			expectedErrno:   ErrnoFault,
			expectedLog: `
--> proxy.fd_seek(fd=4,offset=0,whence=0,result.newoffset=65536)
	--> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=0,result.newoffset=65536)
	<-- EFAULT
<-- 21
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, fdSeekName, uint64(tc.fd), tc.offset, uint64(tc.whence), uint64(tc.resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdSync only tests it is stubbed for GrainLang per #271
func Test_fdSync(t *testing.T) {
	log := requireErrnoNosys(t, fdSyncName, 0)
	require.Equal(t, `
--> proxy.fd_sync(fd=0)
	--> wasi_snapshot_preview1.fd_sync(fd=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_fdTell only tests it is stubbed for GrainLang per #271
func Test_fdTell(t *testing.T) {
	log := requireErrnoNosys(t, fdTellName, 0, 0)
	require.Equal(t, `
--> proxy.fd_tell(fd=0,result.offset=0)
	--> wasi_snapshot_preview1.fd_tell(fd=0,result.offset=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_fdWrite(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenWritableFile(t, tmpDir, pathName)
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

	requireErrno(t, ErrnoSuccess, mod, fdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	require.Equal(t, `
--> proxy.fd_write(fd=4,iovs=1,iovs_len=2,result.nwritten=26)
	--> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2,result.nwritten=26)
		==> wasi_snapshot_preview1.fdWrite(fd=4,iovs=1,iovs_len=2)
		<== (nwritten=6,ESUCCESS)
	<-- ESUCCESS
<-- 0
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

	fd := 1 // stdout
	requireErrno(t, ErrnoSuccess, mod, fdWriteName, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
	require.Equal(t, `
--> proxy.fd_write(fd=1,iovs=1,iovs_len=2,result.nwritten=26)
	--> wasi_snapshot_preview1.fd_write(fd=1,iovs=1,iovs_len=2,result.nwritten=26)
		==> wasi_snapshot_preview1.fdWrite(fd=1,iovs=1,iovs_len=2)
		<== (nwritten=6,ESUCCESS)
	<-- ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdWrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenWritableFile(t, tmpDir, pathName)
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
--> proxy.fd_write(fd=42,iovs=0,iovs_len=1,result.nwritten=0)
	--> wasi_snapshot_preview1.fd_write(fd=42,iovs=0,iovs_len=1,result.nwritten=0)
		==> wasi_snapshot_preview1.fdWrite(fd=42,iovs=0,iovs_len=1)
		<== (nwritten=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          memSize - 2,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_write(fd=4,iovs=65534,iovs_len=1,result.nwritten=0)
	--> wasi_snapshot_preview1.fd_write(fd=4,iovs=65534,iovs_len=1,result.nwritten=0)
		==> wasi_snapshot_preview1.fdWrite(fd=4,iovs=65534,iovs_len=1)
		<== (nwritten=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "out-of-memory reading iovs[0].length",
			fd:            fd,
			iovs:          memSize - 4, // iovs[0].offset was 4 bytes and iovs[0].length next, but not enough mod.Memory()!
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_write(fd=4,iovs=65532,iovs_len=1,result.nwritten=0)
	--> wasi_snapshot_preview1.fd_write(fd=4,iovs=65532,iovs_len=1,result.nwritten=0)
		==> wasi_snapshot_preview1.fdWrite(fd=4,iovs=65532,iovs_len=1)
		<== (nwritten=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "iovs[0].offset is outside memory",
			fd:            fd,
			iovs:          memSize - 5, // iovs[0].offset (where to read "hi") is outside memory.
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_write(fd=4,iovs=65531,iovs_len=1,result.nwritten=0)
	--> wasi_snapshot_preview1.fd_write(fd=4,iovs=65531,iovs_len=1,result.nwritten=0)
		==> wasi_snapshot_preview1.fdWrite(fd=4,iovs=65531,iovs_len=1)
		<== (nwritten=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "length to read exceeds memory by 1",
			fd:            fd,
			iovs:          memSize - 9, // iovs[0].offset (where to read "hi") is in memory, but truncated.
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.fd_write(fd=4,iovs=65527,iovs_len=1,result.nwritten=0)
	--> wasi_snapshot_preview1.fd_write(fd=4,iovs=65527,iovs_len=1,result.nwritten=0)
		==> wasi_snapshot_preview1.fdWrite(fd=4,iovs=65527,iovs_len=1)
		<== (nwritten=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:           "resultNwritten offset is outside memory",
			fd:             fd,
			resultNwritten: memSize, // read was ok, but there wasn't enough memory to write the result.
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.fd_write(fd=4,iovs=0,iovs_len=1,result.nwritten=65536)
	--> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1,result.nwritten=65536)
	<-- EFAULT
<-- 21
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

			requireErrno(t, tc.expectedErrno, mod, fdWriteName, uint64(tc.fd), uint64(tc.iovs), uint64(iovsCount),
				uint64(tc.resultNwritten))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_pathCreateDirectory only tests it is stubbed for GrainLang per #271
func Test_pathCreateDirectory(t *testing.T) {
	log := requireErrnoNosys(t, pathCreateDirectoryName, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_create_directory(fd=0,path=0,path_len=0)
	--> wasi_snapshot_preview1.path_create_directory(fd=0,path=0,path_len=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_pathFilestatGet(t *testing.T) {
	file, dir := "a", "b"
	testFS := fstest.MapFS{
		file:             {Data: make([]byte, 10), ModTime: time.Unix(1667482413, 0)},
		dir:              {Mode: fs.ModeDir, ModTime: time.Unix(1667482413, 0)},
		dir + "/" + file: {Data: make([]byte, 20), ModTime: time.Unix(1667482413, 0)},
	}

	initialMemoryFile := append([]byte{'?'}, file...)
	initialMemoryDir := append([]byte{'?'}, dir...)
	initialMemoryNotExists := []byte{'?', '?'}

	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size()

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS()

	rootFd := uint32(3) // after stderr

	fileFd, err := fsc.OpenFile(file)
	require.NoError(t, err)

	dirFd, err := fsc.OpenFile(dir)
	require.NoError(t, err)

	tests := []struct {
		name                        string
		fd, pathLen, resultFilestat uint32
		memory, expectedMemory      []byte
		expectedErrno               Errno
		expectedLog                 string
	}{
		{
			name:           "file under root",
			fd:             rootFd,
			memory:         initialMemoryFile,
			pathLen:        1,
			resultFilestat: 2,
			expectedMemory: append(
				initialMemoryFile,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				10, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			),
			expectedLog: `
--> proxy.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=2)
	==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=2)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name:           "file under dir",
			fd:             dirFd, // root
			memory:         initialMemoryFile,
			pathLen:        1,
			resultFilestat: 2,
			expectedMemory: append(
				initialMemoryFile,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				4, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				20, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			),
			expectedLog: `
--> proxy.path_filestat_get(fd=5,flags=0,path=1,path_len=1,result.buf=2)
	==> wasi_snapshot_preview1.path_filestat_get(fd=5,flags=0,path=1,path_len=1,result.buf=2)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name:           "dir under root",
			fd:             rootFd,
			memory:         initialMemoryDir,
			pathLen:        1,
			resultFilestat: 2,
			expectedMemory: append(
				initialMemoryDir,
				0, 0, 0, 0, 0, 0, 0, 0, // dev
				0, 0, 0, 0, 0, 0, 0, 0, // ino
				3, 0, 0, 0, 0, 0, 0, 0, // filetype + padding
				1, 0, 0, 0, 0, 0, 0, 0, // nlink
				0, 0, 0, 0, 0, 0, 0, 0, // size
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // atim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // mtim
				0x0, 0x82, 0x13, 0x80, 0x6b, 0x16, 0x24, 0x17, // ctim
			),
			expectedLog: `
--> proxy.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=2)
	==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=2)
	<== ESUCCESS
<-- 0
`,
		},
		{
			name:          "bad FD - not opened",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.path_filestat_get(fd=-1,flags=0,path=1,path_len=0,result.buf=0)
	==> wasi_snapshot_preview1.path_filestat_get(fd=-1,flags=0,path=1,path_len=0,result.buf=0)
	<== EBADF
<-- 8
`,
		},
		{
			name:           "bad FD - not dir",
			fd:             fileFd,
			memory:         initialMemoryFile,
			pathLen:        1,
			resultFilestat: 2,
			expectedErrno:  ErrnoNotdir,
			expectedLog: `
--> proxy.path_filestat_get(fd=4,flags=0,path=1,path_len=1,result.buf=2)
	==> wasi_snapshot_preview1.path_filestat_get(fd=4,flags=0,path=1,path_len=1,result.buf=2)
	<== ENOTDIR
<-- 54
`,
		},
		{
			name:           "path under root doesn't exist",
			fd:             rootFd,
			memory:         initialMemoryNotExists,
			pathLen:        1,
			resultFilestat: 2,
			expectedErrno:  ErrnoNoent,
			expectedLog: `
--> proxy.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=2)
	==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=2)
	<== ENOENT
<-- 44
`,
		},
		{
			name:           "path under dir doesn't exist",
			fd:             dirFd,
			memory:         initialMemoryNotExists,
			pathLen:        1,
			resultFilestat: 2,
			expectedErrno:  ErrnoNoent,
			expectedLog: `
--> proxy.path_filestat_get(fd=5,flags=0,path=1,path_len=1,result.buf=2)
	==> wasi_snapshot_preview1.path_filestat_get(fd=5,flags=0,path=1,path_len=1,result.buf=2)
	<== ENOENT
<-- 44
`,
		},
		{
			name:           "path invalid",
			fd:             dirFd,
			memory:         []byte("?../foo"),
			pathLen:        6,
			resultFilestat: 7,
			expectedErrno:  ErrnoNoent,
			expectedLog: `
--> proxy.path_filestat_get(fd=5,flags=0,path=1,path_len=6,result.buf=7)
	==> wasi_snapshot_preview1.path_filestat_get(fd=5,flags=0,path=1,path_len=6,result.buf=7)
	<== ENOENT
<-- 44
`,
		},
		{
			name:          "path is out of memory",
			fd:            rootFd,
			memory:        initialMemoryFile,
			pathLen:       memorySize,
			expectedErrno: ErrnoNametoolong,
			expectedLog: `
--> proxy.path_filestat_get(fd=3,flags=0,path=1,path_len=65536,result.buf=0)
	==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=0,path=1,path_len=65536,result.buf=0)
	<== ENAMETOOLONG
<-- 37
`,
		},
		{
			name:           "resultFilestat exceeds the maximum valid address by 1",
			fd:             rootFd,
			memory:         initialMemoryFile,
			pathLen:        1,
			resultFilestat: memorySize - 64 + 1,
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=65473)
	==> wasi_snapshot_preview1.path_filestat_get(fd=3,flags=0,path=1,path_len=1,result.buf=65473)
	<== EFAULT
<-- 21
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
			requireErrno(t, tc.expectedErrno, mod, pathFilestatGetName, uint64(tc.fd), uint64(flags), uint64(1), uint64(tc.pathLen), uint64(tc.resultFilestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(0, uint32(len(tc.expectedMemory)))
			require.True(t, ok)
			require.Equal(t, tc.expectedMemory, actual)
		})
	}
}

// Test_pathFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_pathFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, pathFilestatSetTimesName, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_filestat_set_times(fd=0,flags=0,path=0,path_len=0,atim=0,mtim=0,fst_flags=0)
	--> wasi_snapshot_preview1.path_filestat_set_times(fd=0,flags=0,path=0,path_len=0,atim=0,mtim=0,fst_flags=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_pathLink only tests it is stubbed for GrainLang per #271
func Test_pathLink(t *testing.T) {
	log := requireErrnoNosys(t, pathLinkName, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_link(old_fd=0,old_flags=0,old_path=0,old_path_len=0,new_fd=0,new_path=0,new_path_len=0)
	--> wasi_snapshot_preview1.path_link(old_fd=0,old_flags=0,old_path=0,old_path_len=0,new_fd=0,new_path=0,new_path_len=0)
	<-- ENOSYS
<-- 52
`, log)
}

func Test_pathOpen(t *testing.T) {
	rootFD := uint32(3) // after 0, 1, and 2, that are stdin/out/err
	expectedFD := rootFD + 1
	// set up the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	initialMemory := append([]byte{'?'}, pathName...)

	expectedMemory := append(
		initialMemory,
		'?', // `resultOpenedFd` is after this
		byte(expectedFD), 0, 0, 0,
		'?',
	)

	dirflags := uint32(0)
	path := uint32(1)
	pathLen := uint32(len(pathName))
	oflags := uint32(0)
	fsRightsBase := uint64(1)       // ignored: rights were removed from WASI.
	fsRightsInheriting := uint64(2) // ignored: rights were removed from WASI.
	fdflags := uint32(0)
	resultOpenedFd := uint32(len(initialMemory) + 1)

	testFS := fstest.MapFS{pathName: &fstest.MapFile{Mode: os.ModeDir}}
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)

	maskMemory(t, mod, len(expectedMemory))
	ok := mod.Memory().Write(0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, pathOpenName, uint64(rootFD), uint64(dirflags), uint64(path),
		uint64(pathLen), uint64(oflags), fsRightsBase, fsRightsInheriting, uint64(fdflags), uint64(resultOpenedFd))
	require.Equal(t, `
--> proxy.path_open(fd=3,dirflags=0,path=1,path_len=6,oflags=0,fs_rights_base=1,fs_rights_inheriting=2,fdflags=0,result.opened_fd=8)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=1,path_len=6,oflags=0,fs_rights_base=1,fs_rights_inheriting=2,fdflags=0,result.opened_fd=8)
		==> wasi_snapshot_preview1.pathOpen(fd=3,dirflags=0,path=1,path_len=6,oflags=0,fs_rights_base=1,fs_rights_inheriting=2,fdflags=0)
		<== (opened_fd=4,ESUCCESS)
	<-- ESUCCESS
<-- 0
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// verify the file was actually opened
	fsc := mod.(*wasm.CallContext).Sys.FS()
	f, ok := fsc.OpenedFile(expectedFD)
	require.True(t, ok)
	require.Equal(t, pathName, f.Name)
}

func Test_pathOpen_Errors(t *testing.T) {
	validFD := uint32(3) // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	dirName := "wazero"
	fileName := "notdir" // name length as wazero
	testFS := fstest.MapFS{
		dirName:  &fstest.MapFile{Mode: os.ModeDir},
		fileName: &fstest.MapFile{},
	}
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)

	validPath := uint32(0)    // arbitrary offset
	validPathLen := uint32(6) // the length of dirName

	tests := []struct {
		name, pathName                            string
		fd, path, pathLen, oflags, resultOpenedFd uint32
		expectedErrno                             Errno
		expectedLog                               string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
--> proxy.path_open(fd=42,dirflags=0,path=0,path_len=0,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
	--> wasi_snapshot_preview1.path_open(fd=42,dirflags=0,path=0,path_len=0,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
		==> wasi_snapshot_preview1.pathOpen(fd=42,dirflags=0,path=0,path_len=0,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0)
		<== (opened_fd=0,EBADF)
	<-- EBADF
<-- 8
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            validFD,
			path:          mod.Memory().Size(),
			pathLen:       validPathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.path_open(fd=3,dirflags=0,path=65536,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=65536,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
		==> wasi_snapshot_preview1.pathOpen(fd=3,dirflags=0,path=65536,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0)
		<== (opened_fd=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:     "path invalid",
			fd:       validFD,
			pathName: "../foo",
			pathLen:  6,
			// fstest.MapFS returns file not found instead of invalid on invalid path
			expectedErrno: ErrnoNoent,
			expectedLog: `
--> proxy.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
		==> wasi_snapshot_preview1.pathOpen(fd=3,dirflags=0,path=0,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0)
		<== (opened_fd=0,ENOENT)
	<-- ENOENT
<-- 44
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            validFD,
			path:          validPath,
			pathLen:       mod.Memory().Size() + 1, // path is in the valid memory range, but pathLen is out-of-memory for path
			expectedErrno: ErrnoFault,
			expectedLog: `
--> proxy.path_open(fd=3,dirflags=0,path=0,path_len=65537,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=65537,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
		==> wasi_snapshot_preview1.pathOpen(fd=3,dirflags=0,path=0,path_len=65537,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0)
		<== (opened_fd=0,EFAULT)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "no such file exists",
			fd:            validFD,
			pathName:      dirName,
			path:          validPath,
			pathLen:       validPathLen - 1, // this make the path "wazer", which doesn't exit
			expectedErrno: ErrnoNoent,
			expectedLog: `
--> proxy.path_open(fd=3,dirflags=0,path=0,path_len=5,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=5,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
		==> wasi_snapshot_preview1.pathOpen(fd=3,dirflags=0,path=0,path_len=5,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0)
		<== (opened_fd=0,ENOENT)
	<-- ENOENT
<-- 44
`,
		},
		{
			name:           "out-of-memory writing resultOpenedFd",
			fd:             validFD,
			pathName:       dirName,
			path:           validPath,
			pathLen:        validPathLen,
			resultOpenedFd: mod.Memory().Size(), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  ErrnoFault,
			expectedLog: `
--> proxy.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=65536)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=65536)
	<-- EFAULT
<-- 21
`,
		},
		{
			name:          "oflags=directory, but not a directory",
			oflags:        uint32(wasiOflagsDirectory),
			fd:            validFD,
			pathName:      fileName,
			path:          validPath,
			pathLen:       validPathLen,
			expectedErrno: ErrnoNotdir,
			expectedLog: `
--> proxy.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=3,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
	--> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=3,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
		==> wasi_snapshot_preview1.pathOpen(fd=3,dirflags=0,path=0,path_len=6,oflags=3,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0)
		<== (opened_fd=4,ENOTDIR)
	<-- ENOTDIR
<-- 54
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().Write(validPath, []byte(tc.pathName))

			requireErrno(t, tc.expectedErrno, mod, pathOpenName, uint64(tc.fd), uint64(0), uint64(tc.path),
				uint64(tc.pathLen), uint64(tc.oflags), 0, 0, 0, uint64(tc.resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_pathReadlink only tests it is stubbed for GrainLang per #271
func Test_pathReadlink(t *testing.T) {
	log := requireErrnoNosys(t, pathReadlinkName, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_readlink(fd=0,path=0,path_len=0,buf=0,buf_len=0,result.bufused=0)
	--> wasi_snapshot_preview1.path_readlink(fd=0,path=0,path_len=0,buf=0,buf_len=0,result.bufused=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_pathRemoveDirectory only tests it is stubbed for GrainLang per #271
func Test_pathRemoveDirectory(t *testing.T) {
	log := requireErrnoNosys(t, pathRemoveDirectoryName, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_remove_directory(fd=0,path=0,path_len=0)
	--> wasi_snapshot_preview1.path_remove_directory(fd=0,path=0,path_len=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_pathRename only tests it is stubbed for GrainLang per #271
func Test_pathRename(t *testing.T) {
	log := requireErrnoNosys(t, pathRenameName, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_rename(fd=0,old_path=0,old_path_len=0,new_fd=0,new_path=0,new_path_len=0)
	--> wasi_snapshot_preview1.path_rename(fd=0,old_path=0,old_path_len=0,new_fd=0,new_path=0,new_path_len=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_pathSymlink only tests it is stubbed for GrainLang per #271
func Test_pathSymlink(t *testing.T) {
	log := requireErrnoNosys(t, pathSymlinkName, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_symlink(old_path=0,old_path_len=0,fd=0,new_path=0,new_path_len=0)
	--> wasi_snapshot_preview1.path_symlink(old_path=0,old_path_len=0,fd=0,new_path=0,new_path_len=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_pathUnlinkFile only tests it is stubbed for GrainLang per #271
func Test_pathUnlinkFile(t *testing.T) {
	log := requireErrnoNosys(t, pathUnlinkFileName, 0, 0, 0)
	require.Equal(t, `
--> proxy.path_unlink_file(fd=0,path=0,path_len=0)
	--> wasi_snapshot_preview1.path_unlink_file(fd=0,path=0,path_len=0)
	<-- ENOSYS
<-- 52
`, log)
}

func requireOpenFile(t *testing.T, pathName string, data []byte) (api.Module, uint32, *bytes.Buffer, api.Closer) {
	mapFile := &fstest.MapFile{Data: data}
	if data == nil {
		mapFile.Mode = os.ModeDir
	}
	testFS := fstest.MapFS{pathName[1:]: mapFile} // strip the leading slash
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	fsc := mod.(*wasm.CallContext).Sys.FS()
	fd, err := fsc.OpenFile(pathName)
	require.NoError(t, err)
	return mod, fd, log, r
}

// requireOpenWritableFile is temporary until we add the ability to open files for writing.
func requireOpenWritableFile(t *testing.T, tmpDir string, pathName string) (api.Module, uint32, *bytes.Buffer, api.Closer) {
	writeable, testFS := createWriteableFile(t, tmpDir, pathName, []byte{})
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().WithFS(testFS))
	fsc := mod.(*wasm.CallContext).Sys.FS()
	fd, err := fsc.OpenFile(pathName)
	require.NoError(t, err)

	// Swap the read-only file with a writeable one until #390
	f, ok := fsc.OpenedFile(fd)
	require.True(t, ok)
	f.File.Close()
	f.File = writeable

	return mod, fd, log, r
}

// createWriteableFile uses real files when io.Writer tests are needed.
func createWriteableFile(t *testing.T, tmpDir string, pathName string, data []byte) (fs.File, fs.FS) {
	require.NotNil(t, data)
	absolutePath := path.Join(tmpDir, pathName)
	require.NoError(t, os.WriteFile(absolutePath, data, 0o600))

	// open the file for writing in a custom way until #390
	f, err := os.OpenFile(absolutePath, os.O_RDWR, 0o600)
	require.NoError(t, err)
	return f, os.DirFS(tmpDir)
}
