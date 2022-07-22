package wasi_snapshot_preview1

import (
	"bytes"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Test_fdAdvise only tests it is stubbed for GrainLang per #271
func Test_fdAdvise(t *testing.T) {
	log := requireErrnoNosys(t, functionFdAdvise, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_advise(fd=0,offset=0,len=0,result.advice=0)
<-- ENOSYS
`, log)
}

// Test_fdAllocate only tests it is stubbed for GrainLang per #271
func Test_fdAllocate(t *testing.T) {
	log := requireErrnoNosys(t, functionFdAllocate, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_allocate(fd=0,offset=0,len=0)
<-- ENOSYS
`, log)
}

func Test_fdClose(t *testing.T) {
	// fd_close needs to close an open file descriptor. Open two files so that we can tell which is closed.
	path1, path2 := "a", "b"
	testFS := fstest.MapFS{path1: {Data: make([]byte, 0)}, path2: {Data: make([]byte, 0)}}

	mod, r, log := requireModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)

	fdToClose, err := fsc.OpenFile(testCtx, path1)
	require.NoError(t, err)

	fdToKeep, err := fsc.OpenFile(testCtx, path2)
	require.NoError(t, err)

	// Close
	requireErrno(t, ErrnoSuccess, mod, functionFdClose, uint64(fdToClose))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=4)
<== ESUCCESS
`, "\n"+log.String())

	// Verify fdToClose is closed and removed from the opened FDs.
	_, ok := fsc.OpenedFile(testCtx, fdToClose)
	require.False(t, ok)

	// Verify fdToKeep is not closed
	_, ok = fsc.OpenedFile(testCtx, fdToKeep)
	require.True(t, ok)

	log.Reset()
	t.Run("ErrnoBadF for an invalid FD", func(t *testing.T) {
		requireErrno(t, ErrnoBadf, mod, functionFdClose, uint64(42)) // 42 is an arbitrary invalid FD
		require.Equal(t, `
==> wasi_snapshot_preview1.fd_close(fd=42)
<== EBADF
`, "\n"+log.String())
	})
}

// Test_fdDatasync only tests it is stubbed for GrainLang per #271
func Test_fdDatasync(t *testing.T) {
	log := requireErrnoNosys(t, functionFdDatasync, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_datasync(fd=0)
<-- ENOSYS
`, log)
}

func Test_fdFdstatGet(t *testing.T) {
	file, dir := "a", "b"
	testFS := fstest.MapFS{file: {Data: make([]byte, 0)}, dir: {Mode: fs.ModeDir}}

	mod, r, log := requireModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)
	memorySize := mod.Memory().Size(testCtx)

	// open both paths without using WASI
	fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)

	fileFd, err := fsc.OpenFile(testCtx, file)
	require.NoError(t, err)

	dirFd, err := fsc.OpenFile(testCtx, dir)
	require.NoError(t, err)

	tests := []struct {
		name           string
		fd, resultStat uint32
		// TODO: expectedMem
		expectedErrno Errno
		expectedLog   string
	}{
		{
			name: "file",
			fd:   fileFd,
			// TODO: expectedMem for a file
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=4,result.stat=0)
<== ESUCCESS
`,
		},
		{
			name: "dir",
			fd:   dirFd,
			// TODO: expectedMem for a dir
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5,result.stat=0)
<== ESUCCESS
`,
		},
		{
			name:          "bad FD",
			fd:            math.MaxUint32,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=4294967295,result.stat=0)
<== EBADF
`,
		},
		{
			name:       "resultStat exceeds the maximum valid address by 1",
			fd:         dirFd,
			resultStat: memorySize - 24 + 1,
			// TODO: ErrnoFault
			expectedLog: `
==> wasi_snapshot_preview1.fd_fdstat_get(fd=5,result.stat=65513)
<== ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, functionFdFdstatGet, uint64(tc.fd), uint64(tc.resultStat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdFdstatSetFlags only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetFlags(t *testing.T) {
	log := requireErrnoNosys(t, functionFdFdstatSetFlags, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_fdstat_set_flags(fd=0,flags=0)
<-- ENOSYS
`, log)
}

// Test_fdFdstatSetRights only tests it is stubbed for GrainLang per #271
func Test_fdFdstatSetRights(t *testing.T) {
	log := requireErrnoNosys(t, functionFdFdstatSetRights, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_fdstat_set_rights(fd=0,fs_rights_base=0,fs_rights_inheriting=0)
<-- ENOSYS
`, log)
}

// Test_fdFilestatGet only tests it is stubbed for GrainLang per #271
func Test_fdFilestatGet(t *testing.T) {
	log := requireErrnoNosys(t, functionFdFilestatGet, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_filestat_get(fd=0,result.buf=0)
<-- ENOSYS
`, log)
}

// Test_fdFilestatSetSize only tests it is stubbed for GrainLang per #271
func Test_fdFilestatSetSize(t *testing.T) {
	log := requireErrnoNosys(t, functionFdFilestatSetSize, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_filestat_set_size(fd=0,size=0)
<-- ENOSYS
`, log)
}

// Test_fdFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_fdFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, functionFdFilestatSetTimes, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_filestat_set_times(fd=0,atim=0,mtim=0,fst_flags=0)
<-- ENOSYS
`, log)
}

// Test_fdPread only tests it is stubbed for GrainLang per #271
func Test_fdPread(t *testing.T) {
	log := requireErrnoNosys(t, functionFdPread, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_pread(fd=0,iovs=0,iovs_len=0,offset=0,result.nread=0)
<-- ENOSYS
`, log)
}

func Test_fdPrestatGet(t *testing.T) {
	pathName := "/tmp"
	mod, fd, log, r := requireOpenFile(t, pathName, nil)
	defer r.Close(testCtx)

	resultPrestat := uint32(1) // arbitrary offset
	expectedMemory := []byte{
		'?',     // resultPrestat after this
		0,       // 8-bit tag indicating `prestat_dir`, the only available tag
		0, 0, 0, // 3-byte padding
		// the result path length field after this
		byte(len(pathName)), 0, 0, 0, // = in little endian encoding
		'?',
	}

	maskMemory(t, testCtx, mod, len(expectedMemory))

	requireErrno(t, ErrnoSuccess, mod, functionFdPrestatGet, uint64(fd), uint64(resultPrestat))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_prestat_get(fd=4,result.prestat=1)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatGet_Errors(t *testing.T) {
	pathName := "/tmp"
	mod, fd, log, r := requireOpenFile(t, pathName, nil)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
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
==> wasi_snapshot_preview1.fd_prestat_get(fd=42,result.prestat=0)
<== EBADF
`,
		},
		{
			name:          "out-of-memory resultPrestat",
			fd:            fd,
			resultPrestat: memorySize,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_get(fd=4,result.prestat=65536)
<== EFAULT
`,
		},
		// TODO: non pre-opened file == api.ErrnoBadf
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, functionFdPrestatGet, uint64(tc.fd), uint64(tc.resultPrestat))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

func Test_fdPrestatDirName(t *testing.T) {
	pathName := "/tmp"
	mod, fd, log, r := requireOpenFile(t, pathName, nil)
	defer r.Close(testCtx)

	path := uint32(1)    // arbitrary offset
	pathLen := uint32(3) // shorter than len("/tmp") to test the path is written for the length of pathLen
	expectedMemory := []byte{
		'?',
		'/', 't', 'm',
		'?', '?', '?',
	}

	maskMemory(t, testCtx, mod, len(expectedMemory))

	requireErrno(t, ErrnoSuccess, mod, functionFdPrestatDirName, uint64(fd), uint64(path), uint64(pathLen))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=4,path=1,path_len=3)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdPrestatDirName_Errors(t *testing.T) {
	pathName := "/tmp"
	mod, fd, log, r := requireOpenFile(t, pathName, nil)
	defer r.Close(testCtx)

	memorySize := mod.Memory().Size(testCtx)
	validAddress := uint32(0) // Arbitrary valid address as arguments to fd_prestat_dir_name. We chose 0 here.
	pathLen := uint32(len("/tmp"))

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
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=4,path=65536,path_len=4)
<== EFAULT
`,
		},
		{
			name:          "path exceeds the maximum valid address by 1",
			fd:            fd,
			path:          memorySize - pathLen + 1,
			pathLen:       pathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=4,path=65533,path_len=4)
<== EFAULT
`,
		},
		{
			name:          "pathLen exceeds the length of the dir name",
			fd:            fd,
			path:          validAddress,
			pathLen:       pathLen + 1,
			expectedErrno: ErrnoNametoolong,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=4,path=0,path_len=5)
<== ENAMETOOLONG
`,
		},
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			path:          validAddress,
			pathLen:       pathLen,
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_prestat_dir_name(fd=42,path=0,path_len=4)
<== EBADF
`,
		},
		// TODO: non pre-opened file == ErrnoBadf
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, functionFdPrestatDirName, uint64(tc.fd), uint64(tc.path), uint64(tc.pathLen))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdPwrite only tests it is stubbed for GrainLang per #271
func Test_fdPwrite(t *testing.T) {
	log := requireErrnoNosys(t, functionFdPwrite, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_pwrite(fd=0,iovs=0,iovs_len=0,offset=0,result.nwritten=0)
<-- ENOSYS
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
	iovsCount := uint32(2)   // The count of iovs
	resultSize := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',        // resultSize is after this
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, testCtx, mod, len(expectedMemory))

	ok := mod.Memory().Write(testCtx, 0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, functionFdRead, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultSize))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=1,iovs_len=2,result.size=26)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)
}

func Test_fdRead_Errors(t *testing.T) {
	mod, fd, log, r := requireOpenFile(t, "/test_path", []byte("wazero"))
	defer r.Close(testCtx)

	tests := []struct {
		name                            string
		fd, iovs, iovsCount, resultSize uint32
		memory                          []byte
		expectedErrno                   Errno
		expectedLog                     string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=42,iovs=65536,iovs_len=65536,result.size=65536)
<== EBADF
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			iovs:          1,
			memory:        []byte{'?'},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65536,iovs_len=65535,result.size=65535)
<== EFAULT
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
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65532,iovs_len=65532,result.size=65531)
<== EFAULT
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
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65528,iovs_len=65528,result.size=65527)
<== EFAULT
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
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527,result.size=65526)
<== EFAULT
`,
		},
		{
			name: "resultSize offset is outside memory",
			fd:   fd,
			iovs: 1, iovsCount: 1,
			resultSize: 10, // 1 past memory
			memory: []byte{
				'?',        // `iovs` is after this
				9, 0, 0, 0, // = iovs[0].offset
				1, 0, 0, 0, // = iovs[0].length
				'?',
			},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_read(fd=4,iovs=65527,iovs_len=65527,result.size=65536)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			offset := uint32(wasm.MemoryPagesToBytesNum(testMemoryPageSize) - uint64(len(tc.memory)))

			memoryWriteOK := mod.Memory().Write(testCtx, offset, tc.memory)
			require.True(t, memoryWriteOK)

			requireErrno(t, tc.expectedErrno, mod, functionFdRead, uint64(tc.fd), uint64(tc.iovs+offset), uint64(tc.iovsCount+offset), uint64(tc.resultSize+offset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdReaddir only tests it is stubbed for GrainLang per #271
func Test_fdReaddir(t *testing.T) {
	log := requireErrnoNosys(t, functionFdReaddir, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_readdir(fd=0,buf=0,buf_len=0,cookie=0,result.bufused=0)
<-- ENOSYS
`, log)
}

// Test_fdRenumber only tests it is stubbed for GrainLang per #271
func Test_fdRenumber(t *testing.T) {
	log := requireErrnoNosys(t, functionFdRenumber, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_renumber(fd=0,to=0)
<-- ENOSYS
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
				'?',        // resultNewoffset is after this
				4, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=4,whence=0,result.newoffset=1)
<== ESUCCESS
`,
		},
		{
			name:           "SeekCurrent",
			offset:         1, // arbitrary offset
			whence:         io.SeekCurrent,
			expectedOffset: 2, // = 1 (the initial offset of the test file) + 1 (offset)
			expectedMemory: []byte{
				'?',        // resultNewoffset is after this
				2, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=1,whence=1,result.newoffset=1)
<== ESUCCESS
`,
		},
		{
			name:           "SeekEnd",
			offset:         -1, // arbitrary offset, note that offset can be negative
			whence:         io.SeekEnd,
			expectedOffset: 5, // = 6 (the size of the test file with content "wazero") + -1 (offset)
			expectedMemory: []byte{
				'?',        // resultNewoffset is after this
				5, 0, 0, 0, // = expectedOffset
				'?',
			},
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=18446744073709551615,whence=2,result.newoffset=1)
<== ESUCCESS
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			maskMemory(t, testCtx, mod, len(tc.expectedMemory))

			// Since we initialized this file, we know it is a seeker (because it is a MapFile)
			fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)
			f, ok := fsc.OpenedFile(testCtx, fd)
			require.True(t, ok)
			seeker := f.File.(io.Seeker)

			// set the initial offset of the file to 1
			offset, err := seeker.Seek(1, io.SeekStart)
			require.NoError(t, err)
			require.Equal(t, int64(1), offset)

			requireErrno(t, ErrnoSuccess, mod, functionFdSeek, uint64(fd), uint64(tc.offset), uint64(tc.whence), uint64(resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())

			actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(tc.expectedMemory)))
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

	memorySize := mod.Memory().Size(testCtx)

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
==> wasi_snapshot_preview1.fd_seek(fd=42,offset=0,whence=0,result.newoffset=0)
<== EBADF
`,
		},
		{
			name:          "invalid whence",
			fd:            fd,
			whence:        3, // invalid whence, the largest whence io.SeekEnd(2) + 1
			expectedErrno: ErrnoInval,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=3,result.newoffset=0)
<== EINVAL
`,
		},
		{
			name:            "out-of-memory writing resultNewoffset",
			fd:              fd,
			resultNewoffset: memorySize,
			expectedErrno:   ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_seek(fd=4,offset=0,whence=0,result.newoffset=65536)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, functionFdSeek, uint64(tc.fd), tc.offset, uint64(tc.whence), uint64(tc.resultNewoffset))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_fdSync only tests it is stubbed for GrainLang per #271
func Test_fdSync(t *testing.T) {
	log := requireErrnoNosys(t, functionFdSync, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_sync(fd=0)
<-- ENOSYS
`, log)
}

// Test_fdTell only tests it is stubbed for GrainLang per #271
func Test_fdTell(t *testing.T) {
	log := requireErrnoNosys(t, functionFdTell, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.fd_tell(fd=0,result.offset=0)
<-- ENOSYS
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
	iovsCount := uint32(2)   // The count of iovs
	resultSize := uint32(26) // arbitrary offset
	expectedMemory := append(
		initialMemory,
		6, 0, 0, 0, // sum(iovs[...].length) == length of "wazero"
		'?',
	)

	maskMemory(t, testCtx, mod, len(expectedMemory))
	ok := mod.Memory().Write(testCtx, 0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, functionFdWrite, uint64(fd), uint64(iovs), uint64(iovsCount), uint64(resultSize))
	require.Equal(t, `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=1,iovs_len=2,result.size=26)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// Since we initialized this file, we know we can read it by path
	buf, err := os.ReadFile(path.Join(tmpDir, pathName))
	require.NoError(t, err)

	require.Equal(t, []byte("wazero"), buf) // verify the file was actually written
}

func Test_fdWrite_Errors(t *testing.T) {
	tmpDir := t.TempDir() // open before loop to ensure no locking problems.
	pathName := "test_path"
	mod, fd, log, r := requireOpenWritableFile(t, tmpDir, pathName)
	defer r.Close(testCtx)

	// Setup valid test memory
	iovs, iovsCount := uint32(0), uint32(1)
	memory := []byte{
		8, 0, 0, 0, // = iovs[0].offset (where the data "hi" begins)
		2, 0, 0, 0, // = iovs[0].length (how many bytes are in "hi")
		'h', 'i', // iovs[0].length bytes
	}

	tests := []struct {
		name           string
		fd, resultSize uint32
		memory         []byte
		expectedErrno  Errno
		expectedLog    string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=42,iovs=0,iovs_len=1,result.size=0)
<== EBADF
`,
		},
		{
			name:          "out-of-memory reading iovs[0].offset",
			fd:            fd,
			memory:        []byte{},
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1,result.size=0)
<== EFAULT
`,
		},
		{
			name:          "out-of-memory reading iovs[0].length",
			fd:            fd,
			memory:        memory[0:4], // iovs[0].offset was 4 bytes and iovs[0].length next, but not enough mod.Memory()!
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1,result.size=0)
<== EFAULT
`,
		},
		{
			name:          "iovs[0].offset is outside memory",
			fd:            fd,
			memory:        memory[0:8], // iovs[0].offset (where to read "hi") is outside memory.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1,result.size=0)
<== EFAULT
`,
		},
		{
			name:          "length to read exceeds memory by 1",
			fd:            fd,
			memory:        memory[0:9], // iovs[0].offset (where to read "hi") is in memory, but truncated.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1,result.size=0)
<== EFAULT
`,
		},
		{
			name:          "resultSize offset is outside memory",
			fd:            fd,
			memory:        memory,
			resultSize:    uint32(len(memory)), // read was ok, but there wasn't enough memory to write the result.
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.fd_write(fd=4,iovs=0,iovs_len=1,result.size=10)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			mod.Memory().(*wasm.MemoryInstance).Buffer = tc.memory

			requireErrno(t, tc.expectedErrno, mod, functionFdWrite, uint64(tc.fd), uint64(iovs), uint64(iovsCount),
				uint64(tc.resultSize))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_pathCreateDirectory only tests it is stubbed for GrainLang per #271
func Test_pathCreateDirectory(t *testing.T) {
	log := requireErrnoNosys(t, functionPathCreateDirectory, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_create_directory(fd=0,path=0,path_len=0)
<-- ENOSYS
`, log)
}

// Test_pathFilestatGet only tests it is stubbed for GrainLang per #271
func Test_pathFilestatGet(t *testing.T) {
	log := requireErrnoNosys(t, functionPathFilestatGet, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_filestat_get(fd=0,flags=0,path=0,path_len=0,result.buf=0)
<-- ENOSYS
`, log)
}

// Test_pathFilestatSetTimes only tests it is stubbed for GrainLang per #271
func Test_pathFilestatSetTimes(t *testing.T) {
	log := requireErrnoNosys(t, functionPathFilestatSetTimes, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_filestat_set_times(fd=0,flags=0,path=0,path_len=0,atim=0,mtim=0,fst_flags=0)
<-- ENOSYS
`, log)
}

// Test_pathLink only tests it is stubbed for GrainLang per #271
func Test_pathLink(t *testing.T) {
	log := requireErrnoNosys(t, functionPathLink, 0, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_link(old_fd=0,old_flags=0,old_path=0,old_path_len=0,new_fd=0,new_path=0,new_path_len=0)
<-- ENOSYS
`, log)
}

func Test_pathOpen(t *testing.T) {
	rootFD := uint32(3) // after 0, 1, and 2, that are stdin/out/err
	expectedFD := rootFD + 1
	// Setup the initial memory to include the path name starting at an offset.
	pathName := "wazero"
	initialMemory := append([]byte{'?'}, pathName...)

	expectedMemory := append(
		initialMemory,
		'?', // `resultOpenedFd` is after this
		byte(expectedFD), 0, 0, 0,
		'?',
	)

	dirflags := uint32(0)
	pathPtr := uint32(1)
	pathLen := uint32(len(pathName))
	oflags := uint32(0)
	// rights are ignored per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
	fsRightsBase := uint64(1)
	fsRightsInheriting := uint64(2)
	fdflags := uint32(0)
	resultOpenedFd := uint32(len(initialMemory) + 1)

	testFS := fstest.MapFS{pathName: &fstest.MapFile{Mode: os.ModeDir}}
	mod, r, log := requireModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)

	maskMemory(t, testCtx, mod, len(expectedMemory))
	ok := mod.Memory().Write(testCtx, 0, initialMemory)
	require.True(t, ok)

	requireErrno(t, ErrnoSuccess, mod, functionPathOpen, uint64(rootFD), uint64(dirflags), uint64(pathPtr),
		uint64(pathLen), uint64(oflags), fsRightsBase, fsRightsInheriting, uint64(fdflags), uint64(resultOpenedFd))
	require.Equal(t, `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=1,path_len=6,oflags=0,fs_rights_base=1,fs_rights_inheriting=2,fdflags=0,result.opened_fd=8)
<== ESUCCESS
`, "\n"+log.String())

	actual, ok := mod.Memory().Read(testCtx, 0, uint32(len(expectedMemory)))
	require.True(t, ok)
	require.Equal(t, expectedMemory, actual)

	// verify the file was actually opened
	fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)
	f, ok := fsc.OpenedFile(testCtx, expectedFD)
	require.True(t, ok)
	require.Equal(t, pathName, f.Path)
}

func Test_pathOpen_Errors(t *testing.T) {
	validFD := uint32(3) // arbitrary valid fd after 0, 1, and 2, that are stdin/out/err
	pathName := "wazero"
	testFS := fstest.MapFS{pathName: &fstest.MapFile{Mode: os.ModeDir}}
	mod, r, log := requireModule(t, wazero.NewModuleConfig().WithFS(testFS))
	defer r.Close(testCtx)

	validPath := uint32(0)    // arbitrary offset
	validPathLen := uint32(6) // the length of "wazero"
	mod.Memory().Write(testCtx, validPath, []byte(pathName))

	tests := []struct {
		name                                      string
		fd, path, pathLen, oflags, resultOpenedFd uint32
		expectedErrno                             Errno
		expectedLog                               string
	}{
		{
			name:          "invalid fd",
			fd:            42, // arbitrary invalid fd
			expectedErrno: ErrnoBadf,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=42,dirflags=0,path=0,path_len=0,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
<== EBADF
`,
		},
		{
			name:          "out-of-memory reading path",
			fd:            validFD,
			path:          mod.Memory().Size(testCtx),
			pathLen:       validPathLen,
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=65536,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
<== EFAULT
`,
		},
		{
			name:          "out-of-memory reading pathLen",
			fd:            validFD,
			path:          validPath,
			pathLen:       mod.Memory().Size(testCtx) + 1, // path is in the valid memory range, but pathLen is out-of-memory for path
			expectedErrno: ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=65537,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
<== EFAULT
`,
		},
		{
			name:          "no such file exists",
			fd:            validFD,
			path:          validPath,
			pathLen:       validPathLen - 1, // this make the path "wazer", which doesn't exit
			expectedErrno: ErrnoNoent,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=5,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=0)
<== ENOENT
`,
		},
		{
			name:           "out-of-memory writing resultOpenedFd",
			fd:             validFD,
			path:           validPath,
			pathLen:        validPathLen,
			resultOpenedFd: mod.Memory().Size(testCtx), // path and pathLen correctly point to the right path, but where to write the opened FD is outside memory.
			expectedErrno:  ErrnoFault,
			expectedLog: `
==> wasi_snapshot_preview1.path_open(fd=3,dirflags=0,path=0,path_len=6,oflags=0,fs_rights_base=0,fs_rights_inheriting=0,fdflags=0,result.opened_fd=65536)
<== EFAULT
`,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			defer log.Reset()

			requireErrno(t, tc.expectedErrno, mod, functionPathOpen, uint64(tc.fd), uint64(0), uint64(tc.path),
				uint64(tc.pathLen), uint64(tc.oflags), 0, 0, 0, uint64(tc.resultOpenedFd))
			require.Equal(t, tc.expectedLog, "\n"+log.String())
		})
	}
}

// Test_pathReadlink only tests it is stubbed for GrainLang per #271
func Test_pathReadlink(t *testing.T) {
	log := requireErrnoNosys(t, functionPathReadlink, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_readlink(fd=0,path=0,path_len=0,buf=0,buf_len=0,result.bufused=0)
<-- ENOSYS
`, log)
}

// Test_pathRemoveDirectory only tests it is stubbed for GrainLang per #271
func Test_pathRemoveDirectory(t *testing.T) {
	log := requireErrnoNosys(t, functionPathRemoveDirectory, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_remove_directory(fd=0,path=0,path_len=0)
<-- ENOSYS
`, log)
}

// Test_pathRename only tests it is stubbed for GrainLang per #271
func Test_pathRename(t *testing.T) {
	log := requireErrnoNosys(t, functionPathRename, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_rename(fd=0,old_path=0,old_path_len=0,new_fd=0,new_path=0,new_path_len=0)
<-- ENOSYS
`, log)
}

// Test_pathSymlink only tests it is stubbed for GrainLang per #271
func Test_pathSymlink(t *testing.T) {
	log := requireErrnoNosys(t, functionPathSymlink, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_symlink(old_path=0,old_path_len=0,fd=0,new_path=0,new_path_len=0)
<-- ENOSYS
`, log)
}

// Test_pathUnlinkFile only tests it is stubbed for GrainLang per #271
func Test_pathUnlinkFile(t *testing.T) {
	log := requireErrnoNosys(t, functionPathUnlinkFile, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.path_unlink_file(fd=0,path=0,path_len=0)
<-- ENOSYS
`, log)
}

func requireOpenFile(t *testing.T, pathName string, data []byte) (api.Module, uint32, *bytes.Buffer, api.Closer) {
	mapFile := &fstest.MapFile{Data: data}
	if data == nil {
		mapFile.Mode = os.ModeDir
	}
	testFS := fstest.MapFS{pathName[1:]: mapFile} // strip the leading slash
	mod, r, log := requireModule(t, wazero.NewModuleConfig().WithFS(testFS))
	fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)
	fd, err := fsc.OpenFile(testCtx, pathName)
	require.NoError(t, err)
	return mod, fd, log, r
}

// requireOpenWritableFile is temporary until we add the ability to open files for writing.
func requireOpenWritableFile(t *testing.T, tmpDir string, pathName string) (api.Module, uint32, *bytes.Buffer, api.Closer) {
	writeable, testFS := createWriteableFile(t, tmpDir, pathName, []byte{})
	mod, r, log := requireModule(t, wazero.NewModuleConfig().WithFS(testFS))
	fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)
	fd, err := fsc.OpenFile(testCtx, pathName)
	require.NoError(t, err)

	// Swap the read-only file with a writeable one until #390
	f, ok := fsc.OpenedFile(testCtx, fd)
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
