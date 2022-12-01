package wasi_snapshot_preview1

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math"
	"path"
	"syscall"

	"github.com/tetratelabs/wazero/api"
	internalsys "github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionFdAdvise           = "fd_advise"
	functionFdAllocate         = "fd_allocate"
	functionFdClose            = "fd_close"
	functionFdDatasync         = "fd_datasync"
	functionFdFdstatGet        = "fd_fdstat_get"
	functionFdFdstatSetFlags   = "fd_fdstat_set_flags"
	functionFdFdstatSetRights  = "fd_fdstat_set_rights"
	functionFdFilestatGet      = "fd_filestat_get"
	functionFdFilestatSetSize  = "fd_filestat_set_size"
	functionFdFilestatSetTimes = "fd_filestat_set_times"
	functionFdPread            = "fd_pread"
	functionFdPrestatGet       = "fd_prestat_get"
	functionFdPrestatDirName   = "fd_prestat_dir_name"
	functionFdPwrite           = "fd_pwrite"
	functionFdRead             = "fd_read"
	functionFdReaddir          = "fd_readdir"
	functionFdRenumber         = "fd_renumber"
	functionFdSeek             = "fd_seek"
	functionFdSync             = "fd_sync"
	functionFdTell             = "fd_tell"
	functionFdWrite            = "fd_write"

	functionPathCreateDirectory  = "path_create_directory"
	functionPathFilestatGet      = "path_filestat_get"
	functionPathFilestatSetTimes = "path_filestat_set_times"
	functionPathLink             = "path_link"
	functionPathOpen             = "path_open"
	functionPathReadlink         = "path_readlink"
	functionPathRemoveDirectory  = "path_remove_directory"
	functionPathRename           = "path_rename"
	functionPathSymlink          = "path_symlink"
	functionPathUnlinkFile       = "path_unlink_file"
)

// fdAdvise is the WASI function named functionFdAdvise which provides file
// advisory information on a file descriptor.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_advisefd-fd-offset-filesize-len-filesize-advice-advice---errno
var fdAdvise = stubFunction(
	functionFdAdvise,
	[]wasm.ValueType{i32, i64, i64, i32},
	[]string{"fd", "offset", "len", "result.advice"},
)

// fdAllocate is the WASI function named functionFdAllocate which forces the
// allocation of space in a file.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_allocatefd-fd-offset-filesize-len-filesize---errno
var fdAllocate = stubFunction(
	functionFdAllocate,
	[]wasm.ValueType{i32, i64, i64},
	[]string{"fd", "offset", "len"},
)

// fdClose is the WASI function named functionFdClose which closes a file
// descriptor.
//
// # Parameters
//
//   - fd: file descriptor to close
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: the fd was not open.
//
// Note: This is similar to `close` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_close
// and https://linux.die.net/man/3/close
var fdClose = &wasm.HostFunc{
	ExportNames: []string{functionFdClose},
	Name:        functionFdClose,
	ParamTypes:  []api.ValueType{i32},
	ParamNames:  []string{"fd"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdCloseFn),
	},
}

func fdCloseFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	fd := uint32(params[0])

	if ok := sysCtx.FS(ctx).CloseFile(ctx, fd); !ok {
		return ErrnoBadf
	}
	return ErrnoSuccess
}

// fdDatasync is the WASI function named functionFdDatasync which synchronizes
// the data of a file to disk.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_datasyncfd-fd---errno
var fdDatasync = stubFunction(
	functionFdDatasync,
	[]wasm.ValueType{i32},
	[]string{"fd"},
)

// fdFdstatGet is the WASI function named functionFdFdstatGet which returns the
// attributes of a file descriptor.
//
// # Parameters
//
//   - fd: file descriptor to get the fdstat attributes data
//   - resultFdstat: offset to write the result fdstat data
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoFault: `resultFdstat` points to an offset out of memory
//
// fdstat byte layout is 24-byte size, with the following fields:
//   - fs_filetype 1 byte: the file type
//   - fs_flags 2 bytes: the file descriptor flag
//   - 5 pad bytes
//   - fs_right_base 8 bytes: ignored as rights were removed from WASI.
//   - fs_right_inheriting 8 bytes: ignored as rights were removed from WASI.
//
// For example, with a file corresponding with `fd` was a directory (=3) opened
// with `fd_read` right (=1) and no fs_flags (=0), parameter resultFdstat=1,
// this function writes the below to api.Memory:
//
//	                uint16le   padding            uint64le                uint64le
//	       uint8 --+  +--+  +-----------+  +--------------------+  +--------------------+
//	               |  |  |  |           |  |                    |  |                    |
//	     []byte{?, 3, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0}
//	resultFdstat --^  ^-- fs_flags         ^-- fs_right_base       ^-- fs_right_inheriting
//	               |
//	               +-- fs_filetype
//
// Note: fdFdstatGet returns similar flags to `fsync(fd, F_GETFL)` in POSIX, as
// well as additional fields.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fdstat
// and https://linux.die.net/man/3/fsync
var fdFdstatGet = &wasm.HostFunc{
	ExportNames: []string{functionFdFdstatGet},
	Name:        functionFdFdstatGet,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"fd", "result.stat"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdFdstatGetFn),
	},
}

func fdFdstatGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	// TODO: actually write the fdstat!
	fd, _ := uint32(params[0]), uint32(params[1])

	if _, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
		return ErrnoBadf
	}
	return ErrnoSuccess
}

// fdFdstatSetFlags is the WASI function named functionFdFdstatSetFlags which
// adjusts the flags associated with a file descriptor.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_flagsfd-fd-flags-fdflags---errnoand is stubbed for GrainLang per #271
var fdFdstatSetFlags = stubFunction(
	functionFdFdstatSetFlags,
	[]wasm.ValueType{i32, i32},
	[]string{"fd", "flags"},
)

// fdFdstatSetRights will not be implemented as rights were removed from WASI.
//
// See https://github.com/bytecodealliance/wasmtime/pull/4666
var fdFdstatSetRights = stubFunction(
	functionFdFdstatSetRights,
	[]wasm.ValueType{i32, i64, i64},
	[]string{"fd", "fs_rights_base", "fs_rights_inheriting"},
)

// fdFilestatGet is the WASI function named functionFdFilestatGet which returns
// the stat attributes of an open file.
//
// # Parameters
//
//   - fd: file descriptor to get the filestat attributes data for
//   - resultFilestat: offset to write the result filestat data
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoIo: could not stat `fd` on filesystem
//   - ErrnoFault: `resultFilestat` points to an offset out of memory
//
// filestat byte layout is 64-byte size, with the following fields:
//   - dev 8 bytes: the device ID of device containing the file
//   - ino 8 bytes: the file serial number
//   - filetype 1 byte: the type of the file
//   - 7 pad bytes
//   - nlink 8 bytes: number of hard links to the file
//   - size 8 bytes: for regular files, the file size in bytes. For symbolic links, the length in bytes of the pathname contained in the symbolic link
//   - atim 8 bytes: ast data access timestamp
//   - mtim 8 bytes: last data modification timestamp
//   - ctim 8 bytes: ast file status change timestamp
//
// For example, with a regular file this function writes the below to api.Memory:
//
//	                                                             uint8 --+
//		                         uint64le                uint64le        |        padding               uint64le                uint64le                         uint64le                               uint64le                             uint64le
//		                 +--------------------+  +--------------------+  |  +-----------------+  +--------------------+  +-----------------------+  +----------------------------------+  +----------------------------------+  +----------------------------------+
//		                 |                    |  |                    |  |  |                 |  |                    |  |                       |  |                                  |  |                                  |  |                                  |
//		          []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 117, 80, 0, 0, 0, 0, 0, 0, 160, 153, 212, 128, 110, 221, 35, 23, 160, 153, 212, 128, 110, 221, 35, 23, 160, 153, 212, 128, 110, 221, 35, 23}
//		resultFilestat   ^-- dev                 ^-- ino                 ^                       ^-- nlink               ^-- size                   ^-- atim                              ^-- mtim                              ^-- ctim
//		                                                                 |
//		                                                                 +-- filetype
//
// The following properties of filestat are not implemented:
//   - dev: not supported by Golang FS
//   - ino: not supported by Golang FS
//   - nlink: not supported by Golang FS
//   - atime: not supported by Golang FS, we use mtim for this
//   - ctim: not supported by Golang FS, we use mtim for this
//
// Note: This is similar to `fstat` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_getfd-fd---errno-filestat
// and https://linux.die.net/man/3/fstat
var fdFilestatGet = &wasm.HostFunc{
	ExportNames: []string{functionFdFilestatGet},
	Name:        functionFdFilestatGet,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"fd", "result.buf"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdFilestatGetFn),
	},
}

type wasiFiletype uint8

const (
	wasiFiletypeUnknown wasiFiletype = iota
	wasiFiletypeBlockDevice
	wasiFiletypeCharacterDevice
	wasiFiletypeDirectory
	wasiFiletypeRegularFile
	wasiFiletypeSocketDgram
	wasiFiletypeSocketStream
	wasiFiletypeSymbolicLink
)

func fdFilestatGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	return fdFilestatGetFunc(ctx, mod, uint32(params[0]), uint32(params[1]))
}

func fdFilestatGetFunc(ctx context.Context, mod api.Module, fd, resultBuf uint32) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	file, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd)
	if !ok {
		return ErrnoBadf
	}

	fileStat, err := file.File.Stat()
	if err != nil {
		return ErrnoIo
	}

	fileMode := fileStat.Mode()

	wasiFileMode := wasiFiletypeUnknown
	if fileMode&fs.ModeDevice != 0 {
		wasiFileMode = wasiFiletypeBlockDevice
	} else if fileMode&fs.ModeCharDevice != 0 {
		wasiFileMode = wasiFiletypeCharacterDevice
	} else if fileMode&fs.ModeDir != 0 {
		wasiFileMode = wasiFiletypeDirectory
	} else if fileMode&fs.ModeType == 0 {
		wasiFileMode = wasiFiletypeRegularFile
	} else if fileMode&fs.ModeSymlink != 0 {
		wasiFileMode = wasiFiletypeSymbolicLink
	}

	buf, ok := mod.Memory().Read(ctx, resultBuf, 64)
	if !ok {
		return ErrnoFault
	}

	buf[16] = uint8(wasiFileMode)
	size := uint64(fileStat.Size())
	binary.LittleEndian.PutUint64(buf[32:], size)
	mtim := uint64(fileStat.ModTime().UnixNano())
	binary.LittleEndian.PutUint64(buf[40:], mtim)
	binary.LittleEndian.PutUint64(buf[48:], mtim)
	binary.LittleEndian.PutUint64(buf[56:], mtim)

	return ErrnoSuccess
}

// fdFilestatSetSize is the WASI function named functionFdFilestatSetSize which
// adjusts the size of an open file.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_set_sizefd-fd-size-filesize---errno
var fdFilestatSetSize = stubFunction(
	functionFdFilestatSetSize,
	[]wasm.ValueType{i32, i64},
	[]string{"fd", "size"},
)

// fdFilestatSetTimes is the WASI function named functionFdFilestatSetTimes
// which adjusts the times of an open file.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_set_timesfd-fd-atim-timestamp-mtim-timestamp-fst_flags-fstflags---errno
var fdFilestatSetTimes = stubFunction(
	functionFdFilestatSetTimes,
	[]wasm.ValueType{i32, i64, i64, i32},
	[]string{"fd", "atim", "mtim", "fst_flags"},
)

// fdPread is the WASI function named functionFdPread which reads from a file
// descriptor, without using and updating the file descriptor's offset.
//
// Except for handling offset, this implementation is identical to fdRead.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_preadfd-fd-iovs-iovec_array-offset-filesize---errno-size
var fdPread = &wasm.HostFunc{
	ExportNames: []string{functionFdPread},
	Name:        functionFdPread,
	ParamTypes:  []api.ValueType{i32, i32, i32, i64, i32},
	ParamNames:  []string{"fd", "iovs", "iovs_len", "offset", "result.size"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdPreadFn),
	},
}

func fdPreadFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	return fdReadOrPread(ctx, mod, params, true)
}

// fdPrestatGet is the WASI function named functionFdPrestatGet which returns
// the prestat data of a file descriptor.
//
// # Parameters
//
//   - fd: file descriptor to get the prestat
//   - resultPrestat: offset to write the result prestat data
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid or the `fd` is not a pre-opened directory
//   - ErrnoFault: `resultPrestat` points to an offset out of memory
//
// prestat byte layout is 8 bytes, beginning with an 8-bit tag and 3 pad bytes.
// The only valid tag is `prestat_dir`, which is tag zero. This simplifies the
// byte layout to 4 empty bytes followed by the uint32le encoded path length.
//
// For example, the directory name corresponding with `fd` was "/tmp" and
// parameter resultPrestat=1, this function writes the below to api.Memory:
//
//	                   padding   uint32le
//	        uint8 --+  +-----+  +--------+
//	                |  |     |  |        |
//	      []byte{?, 0, 0, 0, 0, 4, 0, 0, 0, ?}
//	resultPrestat --^           ^
//	          tag --+           |
//	                            +-- size in bytes of the string "/tmp"
//
// See fdPrestatDirName and
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#prestat
var fdPrestatGet = &wasm.HostFunc{
	ExportNames: []string{functionFdPrestatGet},
	Name:        functionFdPrestatGet,
	ParamTypes:  []api.ValueType{i32, i32},
	ParamNames:  []string{"fd", "result.prestat"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdPrestatGetFn),
	},
}

func fdPrestatGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	fd, resultPrestat := uint32(params[0]), uint32(params[1])

	entry, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd)
	if !ok {
		return ErrnoBadf
	}

	// Zero-value 8-bit tag, and 3-byte zero-value paddings, which is uint32le(0) in short.
	if !mod.Memory().WriteUint32Le(ctx, resultPrestat, uint32(0)) {
		return ErrnoFault
	}

	// Write the length of the directory name at offset 4.
	if !mod.Memory().WriteUint32Le(ctx, resultPrestat+4, uint32(len(entry.Path))) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// fdPrestatDirName is the WASI function named functionFdPrestatDirName which
// returns the path of the pre-opened directory of a file descriptor.
//
// # Parameters
//
//   - fd: file descriptor to get the path of the pre-opened directory
//   - path: offset in api.Memory to write the result path
//   - pathLen: count of bytes to write to `path`
//   - This should match the uint32le fdPrestatGet writes to offset
//     `resultPrestat`+4
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoFault: `path` points to an offset out of memory
//   - ErrnoNametoolong: `pathLen` is longer than the actual length of the result
//
// For example, the directory name corresponding with `fd` was "/tmp" and
// # Parameters path=1 pathLen=4 (correct), this function will write the below to
// api.Memory:
//
//	               pathLen
//	           +--------------+
//	           |              |
//	[]byte{?, '/', 't', 'm', 'p', ?}
//	    path --^
//
// See fdPrestatGet
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_dir_name
var fdPrestatDirName = &wasm.HostFunc{
	ExportNames: []string{functionFdPrestatDirName},
	Name:        functionFdPrestatDirName,
	ParamTypes:  []api.ValueType{i32, i32, i32},
	ParamNames:  []string{"fd", "path", "path_len"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdPrestatDirNameFn),
	},
}

func fdPrestatDirNameFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	fd, path, pathLen := uint32(params[0]), uint32(params[1]), uint32(params[2])

	f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd)
	if !ok {
		return ErrnoBadf
	}

	// Some runtimes may have another semantics. See /RATIONALE.md
	if uint32(len(f.Path)) < pathLen {
		return ErrnoNametoolong
	}

	// TODO: fdPrestatDirName may have to return ErrnoNotdir if the type of the
	// prestat data of `fd` is not a PrestatDir.
	if !mod.Memory().Write(ctx, path, []byte(f.Path)[:pathLen]) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// fdPwrite is the WASI function named functionFdPwrite which writes to a file
// descriptor, without using and updating the file descriptor's offset.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_pwritefd-fd-iovs-ciovec_array-offset-filesize---errno-size
var fdPwrite = stubFunction(functionFdPwrite,
	[]wasm.ValueType{i32, i32, i32, i64, i32},
	[]string{"fd", "iovs", "iovs_len", "offset", "result.nwritten"},
)

// fdRead is the WASI function named functionFdRead which reads from a file
// descriptor.
//
// # Parameters
//
//   - fd: an opened file descriptor to read data from
//   - iovs: offset in api.Memory to read offset, size pairs representing where
//     to write file data
//   - Both offset and length are encoded as uint32le
//   - iovsCount: count of memory offset, size pairs to read sequentially
//     starting at iovs
//   - resultSize: offset in api.Memory to write the number of bytes read
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoFault: `iovs` or `resultSize` point to an offset out of memory
//   - ErrnoIo: a file system error
//
// For example, this function needs to first read `iovs` to determine where
// to write contents. If parameters iovs=1 iovsCount=2, this function reads two
// offset/length pairs from api.Memory:
//
//	                  iovs[0]                  iovs[1]
//	          +---------------------+   +--------------------+
//	          | uint32le    uint32le|   |uint32le    uint32le|
//	          +---------+  +--------+   +--------+  +--------+
//	          |         |  |        |   |        |  |        |
//	[]byte{?, 18, 0, 0, 0, 4, 0, 0, 0, 23, 0, 0, 0, 2, 0, 0, 0, ?... }
//	   iovs --^            ^            ^           ^
//	          |            |            |           |
//	 offset --+   length --+   offset --+  length --+
//
// If the contents of the `fd` parameter was "wazero" (6 bytes) and parameter
// resultSize=26, this function writes the below to api.Memory:
//
//	                    iovs[0].length        iovs[1].length
//	                   +--------------+       +----+       uint32le
//	                   |              |       |    |      +--------+
//	[]byte{ 0..16, ?, 'w', 'a', 'z', 'e', ?, 'r', 'o', ?, 6, 0, 0, 0 }
//	  iovs[0].offset --^                      ^           ^
//	                         iovs[1].offset --+           |
//	                                         resultSize --+
//
// Note: This is similar to `readv` in POSIX. https://linux.die.net/man/3/readv
//
// See fdWrite
// and https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_read
var fdRead = &wasm.HostFunc{
	ExportNames: []string{functionFdRead},
	Name:        functionFdRead,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32},
	ParamNames:  []string{"fd", "iovs", "iovs_len", "result.size"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdReadFn),
	},
}

func fdReadFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	return fdReadOrPread(ctx, mod, params, false)
}

func fdReadOrPread(ctx context.Context, mod api.Module, params []uint64, isPread bool) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	mem := mod.Memory()
	fd := uint32(params[0])
	iovs := uint32(params[1])
	iovsCount := uint32(params[2])

	var offset int64
	var resultSize uint32
	if isPread {
		offset = int64(params[3])
		resultSize = uint32(params[4])
	} else {
		resultSize = uint32(params[3])
	}

	r := internalsys.FdReader(ctx, sysCtx, fd)
	if r == nil {
		return ErrnoBadf
	}

	if isPread {
		if s, ok := r.(io.Seeker); ok {
			if _, err := s.Seek(offset, io.SeekStart); err != nil {
				return ErrnoFault
			}
		} else {
			return ErrnoInval
		}
	}

	var nread uint32
	for i := uint32(0); i < iovsCount; i++ {
		iov := iovs + i*8
		offset, ok := mem.ReadUint32Le(ctx, iov)
		if !ok {
			return ErrnoFault
		}
		l, ok := mem.ReadUint32Le(ctx, iov+4)
		if !ok {
			return ErrnoFault
		}
		b, ok := mem.Read(ctx, offset, l)
		if !ok {
			return ErrnoFault
		}

		n, err := r.Read(b)
		nread += uint32(n)

		shouldContinue, errno := fdRead_shouldContinueRead(uint32(n), l, err)
		if errno != ErrnoSuccess {
			return errno
		} else if !shouldContinue {
			break
		}
	}
	if !mem.WriteUint32Le(ctx, resultSize, nread) {
		return ErrnoFault
	} else {
		return ErrnoSuccess
	}
}

// fdRead_shouldContinueRead decides whether to continue reading the next iovec
// based on the amount read (n/l) and a possible error returned from io.Reader.
//
// Note: When there are both bytes read (n) and an error, this continues.
// See /RATIONALE.md "Why ignore the error returned by io.Reader when n > 1?"
func fdRead_shouldContinueRead(n, l uint32, err error) (bool, Errno) {
	if errors.Is(err, io.EOF) {
		return false, ErrnoSuccess // EOF isn't an error, and we shouldn't continue.
	} else if err != nil && n == 0 {
		return false, ErrnoIo
	} else if err != nil {
		return false, ErrnoSuccess // Allow the caller to process n bytes.
	}
	// Continue reading, unless there's a partial read or nothing to read.
	return n == l && n != 0, ErrnoSuccess
}

// fdReaddir is the WASI function named functionFdReaddir which reads directory
// entries from a directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_readdirfd-fd-buf-pointeru8-buf_len-size-cookie-dircookie---errno-size
var fdReaddir = &wasm.HostFunc{
	ExportNames: []string{functionFdReaddir},
	Name:        functionFdReaddir,
	ParamTypes:  []wasm.ValueType{i32, i32, i32, i64, i32},
	ParamNames:  []string{"fd", "buf", "buf_len", "cookie", "result.bufused"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdReaddirFn),
	},
}

func fdReaddirFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	fd := uint32(params[0])
	buf := uint32(params[1])
	bufLen := uint32(params[2])
	// We control the value of the cookie, and it should never be negative.
	// However, we coerce it to signed to ensure the caller doesn't manipulate
	// it in such a way that becomes negative.
	cookie := int64(params[3])
	resultBufused := uint32(params[4])

	// Validate the FD is a directory
	rd, dir, errno := openedDir(ctx, mod, fd)
	if errno != ErrnoSuccess {
		return errno
	}

	// expect a cookie only if we are continuing a read.
	if cookie == 0 && dir.CountRead > 0 {
		return ErrnoInval // invalid as a cookie is minimally one.
	}

	// First, determine the maximum directory entries that can be encoded as
	// dirents. The total size is direntSize(24) + nameSize, for each file.
	// Since a zero-length file name is invalid, the minimum size entry is
	// 25 (direntSize + 1 character).
	maxDirEntries := int(bufLen/direntSize + 1)

	// While unlikely maxDirEntries will fit into bufLen, add one more just in
	// case, as we need to know if we hit the end of the directory or not to
	// write the correct bufused (e.g. == bufLen unless EOF).
	//	>> If less than the size of the read buffer, the end of the
	//	>> directory has been reached.
	maxDirEntries += 1

	// The host keeps state for any unread entries from the prior call because
	// we cannot seek to a previous directory position. Collect these entries.
	entries, errno := lastDirEntries(dir, cookie)
	if errno != ErrnoSuccess {
		return errno
	}

	// Check if we have maxDirEntries, and read more from the FS as needed.
	if entryCount := len(entries); entryCount < maxDirEntries {
		if l, err := rd.ReadDir(maxDirEntries - entryCount); err != io.EOF {
			if err != nil {
				return ErrnoIo
			}
			dir.CountRead += uint64(len(l))
			entries = append(entries, l...)
			// Replace the cache with up to maxDirEntries, starting at cookie.
			dir.Entries = entries
		}
	}

	mem := mod.Memory()

	// Determine how many dirents we can write, excluding a potentially
	// truncated entry.
	bufused, direntCount, writeTruncatedEntry := maxDirents(entries, bufLen)

	// Now, write entries to the underlying buffer.
	if bufused > 0 {

		// d_next is the index of the next file in the list, so it should
		// always be one higher than the requested cookie.
		d_next := uint64(cookie + 1)
		// ^^ yes this can overflow to negative, which means our implementation
		// doesn't support writing greater than max int64 entries.

		dirents, ok := mem.Read(ctx, buf, bufused)
		if !ok {
			return ErrnoFault
		}

		writeDirents(entries, direntCount, writeTruncatedEntry, dirents, d_next)
	}

	if !mem.WriteUint32Le(ctx, resultBufused, bufused) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

const largestDirent = int64(math.MaxUint32 - direntSize)

// lastDirEntries is broken out from fdReaddirFn for testability.
func lastDirEntries(dir *internalsys.ReadDir, cookie int64) (entries []fs.DirEntry, errno Errno) {
	if cookie < 0 {
		errno = ErrnoInval // invalid as we will never send a negative cookie.
		return
	}

	entryCount := int64(len(dir.Entries))
	if entryCount == 0 { // there was no prior call
		if cookie != 0 {
			errno = ErrnoInval // invalid as we haven't sent that cookie
		}
		return
	}

	// Get the first absolute position in our window of results
	firstPos := int64(dir.CountRead) - entryCount
	cookiePos := cookie - firstPos

	switch {
	case cookiePos < 0: // cookie is asking for results outside our window.
		errno = ErrnoNosys // we can't implement directory seeking backwards.
	case cookiePos == 0: // cookie is asking for the next page.
	case cookiePos > entryCount:
		errno = ErrnoInval // invalid as we read that far, yet.
	case cookiePos > 0: // truncate so to avoid large lists.
		entries = dir.Entries[cookiePos:]
	default:
		entries = dir.Entries
	}
	if len(entries) == 0 {
		entries = nil
	}
	return
}

// direntSize is the size of the dirent struct, which should be followed by the
// length of a file name.
const direntSize = uint32(24)

// maxDirents returns the maximum count and total entries that can fit in
// maxLen bytes.
//
// truncatedEntryLen is the amount of bytes past bufLen needed to write the
// next entry. We have to return bufused == bufLen unless the directory is
// exhausted.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_readdir
// See https://github.com/WebAssembly/wasi-libc/blob/659ff414560721b1660a19685110e484a081c3d4/libc-bottom-half/cloudlibc/src/libc/dirent/readdir.c#L44
func maxDirents(entries []fs.DirEntry, bufLen uint32) (bufused, direntCount uint32, writeTruncatedEntry bool) {
	lenRemaining := bufLen
	for _, e := range entries {
		if lenRemaining < direntSize {
			// We don't have enough space in bufLen for another struct,
			// entry. A caller who wants more will retry.

			// bufused == bufLen means more entries exist, which is the case
			// when the dirent is larger than bytes remaining.
			bufused = bufLen
			break
		}

		// use int64 to guard against huge filenames
		nameLen := int64(len(e.Name()))
		var entryLen uint32

		// Check to see if direntSize + nameLen overflows, or if it would be
		// larger than possible to encode.
		if el := int64(direntSize) + nameLen; el < 0 || el > largestDirent {
			// panic, as testing is difficult. ex we would have to extract a
			// function to get size of a string or allocate a 2^32 size one!
			panic("invalid filename: too large")
		} else { // we know this can fit into a uint32
			entryLen = uint32(el)
		}

		if entryLen > lenRemaining {
			// We haven't room to write the entry, and docs say to write the
			// header. This helps especially when there is an entry with a very
			// long filename. Ex if bufLen is 4096 and the filename is 4096,
			// we need to write direntSize(24) + 4096 bytes to write the entry.
			// In this case, we only write up to direntSize(24) to allow the
			// caller to resize.

			// bufused == bufLen means more entries exist, which is the case
			// when the next entry is larger than bytes remaining.
			bufused = bufLen

			// We do have enough space to write the header, this value will be
			// passed on to writeDirents to only write the header for this entry.
			writeTruncatedEntry = true
			break
		}

		// This won't go negative because we checked entryLen <= lenRemaining.
		lenRemaining -= entryLen
		bufused += entryLen
		direntCount++
	}
	return
}

// writeDirents writes the directory entries to the buffer, which is pre-sized
// based on maxDirents.	truncatedEntryLen means write one past entryCount,
// without its name. See maxDirents for why
func writeDirents(
	entries []fs.DirEntry,
	entryCount uint32,
	writeTruncatedEntry bool,
	dirents []byte,
	d_next uint64,
) {
	pos, i := uint32(0), uint32(0)
	for ; i < entryCount; i++ {
		e := entries[i]
		nameLen := uint32(len(e.Name()))

		writeDirent(dirents[pos:], d_next, nameLen, e.IsDir())
		pos += direntSize

		copy(dirents[pos:], e.Name())
		pos += nameLen
		d_next++
	}

	if !writeTruncatedEntry {
		return
	}

	// Write a dirent without its name
	dirent := make([]byte, direntSize)
	e := entries[i]
	writeDirent(dirent, d_next, uint32(len(e.Name())), e.IsDir())

	// Potentially truncate it
	copy(dirents[pos:], dirent)
}

// writeDirent writes direntSize bytes
func writeDirent(buf []byte, dNext uint64, dNamlen uint32, dType bool) {
	binary.LittleEndian.PutUint64(buf, dNext)        // d_next
	binary.LittleEndian.PutUint64(buf[8:], 0)        // no d_ino
	binary.LittleEndian.PutUint32(buf[16:], dNamlen) // d_namlen

	filetype := wasiFiletypeRegularFile
	if dType {
		filetype = wasiFiletypeDirectory
	}
	binary.LittleEndian.PutUint32(buf[20:], uint32(filetype)) //  d_type
}

// openedDir returns the directory and ErrnoSuccess if the fd points to a readable directory.
func openedDir(ctx context.Context, mod api.Module, fd uint32) (fs.ReadDirFile, *internalsys.ReadDir, Errno) {
	fsc := mod.(*wasm.CallContext).Sys.FS(ctx)
	if f, ok := fsc.OpenedFile(ctx, fd); !ok {
		return nil, nil, ErrnoBadf
	} else if d, ok := f.File.(fs.ReadDirFile); !ok {
		return nil, nil, ErrnoNotdir
	} else {
		if f.ReadDir == nil {
			f.ReadDir = &internalsys.ReadDir{}
		}
		return d, f.ReadDir, ErrnoSuccess
	}
}

// fdRenumber is the WASI function named functionFdRenumber which atomically
// replaces a file descriptor by renumbering another file descriptor.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_renumberfd-fd-to-fd---errno
var fdRenumber = stubFunction(
	functionFdRenumber,
	[]wasm.ValueType{i32, i32},
	[]string{"fd", "to"},
)

// fdSeek is the WASI function named functionFdSeek which moves the offset of a
// file descriptor.
//
// # Parameters
//
//   - fd: file descriptor to move the offset of
//   - offset: signed int64, which is encoded as uint64, input argument to
//     `whence`, which results in a new offset
//   - whence: operator that creates the new offset, given `offset` bytes
//   - If io.SeekStart, new offset == `offset`.
//   - If io.SeekCurrent, new offset == existing offset + `offset`.
//   - If io.SeekEnd, new offset == file size of `fd` + `offset`.
//   - resultNewoffset: offset in api.Memory to write the new offset to,
//     relative to start of the file
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoFault: `resultNewoffset` points to an offset out of memory
//   - ErrnoInval: `whence` is an invalid value
//   - ErrnoIo: a file system error
//
// For example, if fd 3 is a file with offset 0, and parameters fd=3, offset=4,
// whence=0 (=io.SeekStart), resultNewOffset=1, this function writes the below
// to api.Memory:
//
//	                         uint64le
//	                  +--------------------+
//	                  |                    |
//	        []byte{?, 4, 0, 0, 0, 0, 0, 0, 0, ? }
//	resultNewoffset --^
//
// Note: This is similar to `lseek` in POSIX. https://linux.die.net/man/3/lseek
//
// See io.Seeker
// and https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_seek
var fdSeek = &wasm.HostFunc{
	ExportNames: []string{functionFdSeek},
	Name:        functionFdSeek,
	ParamTypes:  []api.ValueType{i32, i64, i32, i32},
	ParamNames:  []string{"fd", "offset", "whence", "result.newoffset"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdSeekFn),
	},
}

func fdSeekFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	fd := uint32(params[0])
	offset := params[1]
	whence := uint32(params[2])
	resultNewoffset := uint32(params[3])

	var seeker io.Seeker
	// Check to see if the file descriptor is available
	if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok || f.File == nil {
		return ErrnoBadf
		// fs.FS doesn't declare io.Seeker, but implementations such as os.File implement it.
	} else if seeker, ok = f.File.(io.Seeker); !ok {
		return ErrnoBadf
	}

	if whence > io.SeekEnd /* exceeds the largest valid whence */ {
		return ErrnoInval
	}

	newOffset, err := seeker.Seek(int64(offset), int(whence))
	if err != nil {
		return ErrnoIo
	}
	if !mod.Memory().WriteUint64Le(ctx, resultNewoffset, uint64(newOffset)) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// fdSync is the WASI function named functionFdSync which synchronizes the data
// and metadata of a file to disk.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_syncfd-fd---errno
var fdSync = stubFunction(
	functionFdSync,
	[]wasm.ValueType{i32},
	[]string{"fd"},
)

// fdTell is the WASI function named functionFdTell which returns the current
// offset of a file descriptor.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_tellfd-fd---errno-filesize
var fdTell = stubFunction(
	functionFdTell,
	[]wasm.ValueType{i32, i32},
	[]string{"fd", "result.offset"},
)

// fdWrite is the WASI function named functionFdWrite which writes to a file
// descriptor.
//
// # Parameters
//
//   - fd: an opened file descriptor to write data to
//   - iovs: offset in api.Memory to read offset, size pairs representing the
//     data to write to `fd`
//   - Both offset and length are encoded as uint32le.
//   - iovsCount: count of memory offset, size pairs to read sequentially
//     starting at iovs
//   - resultSize: offset in api.Memory to write the number of bytes written
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoFault: `iovs` or `resultSize` point to an offset out of memory
//   - ErrnoIo: a file system error
//
// For example, this function needs to first read `iovs` to determine what to
// write to `fd`. If parameters iovs=1 iovsCount=2, this function reads two
// offset/length pairs from api.Memory:
//
//	                  iovs[0]                  iovs[1]
//	          +---------------------+   +--------------------+
//	          | uint32le    uint32le|   |uint32le    uint32le|
//	          +---------+  +--------+   +--------+  +--------+
//	          |         |  |        |   |        |  |        |
//	[]byte{?, 18, 0, 0, 0, 4, 0, 0, 0, 23, 0, 0, 0, 2, 0, 0, 0, ?... }
//	   iovs --^            ^            ^           ^
//	          |            |            |           |
//	 offset --+   length --+   offset --+  length --+
//
// This function reads those chunks api.Memory into the `fd` sequentially.
//
//	                    iovs[0].length        iovs[1].length
//	                   +--------------+       +----+
//	                   |              |       |    |
//	[]byte{ 0..16, ?, 'w', 'a', 'z', 'e', ?, 'r', 'o', ? }
//	  iovs[0].offset --^                      ^
//	                         iovs[1].offset --+
//
// Since "wazero" was written, if parameter resultSize=26, this function writes
// the below to api.Memory:
//
//	                   uint32le
//	                  +--------+
//	                  |        |
//	[]byte{ 0..24, ?, 6, 0, 0, 0', ? }
//	     resultSize --^
//
// Note: This is similar to `writev` in POSIX. https://linux.die.net/man/3/writev
//
// See fdRead
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#ciovec
// and https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_write
var fdWrite = &wasm.HostFunc{
	ExportNames: []string{functionFdWrite},
	Name:        functionFdWrite,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32},
	ParamNames:  []string{"fd", "iovs", "iovs_len", "result.size"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(fdWriteFn),
	},
}

func fdWriteFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	fd := uint32(params[0])
	iovs := uint32(params[1])
	iovsCount := uint32(params[2])
	resultSize := uint32(params[3])

	sysCtx := mod.(*wasm.CallContext).Sys
	writer := internalsys.FdWriter(ctx, sysCtx, fd)
	if writer == nil {
		return ErrnoBadf
	}

	var err error
	var nwritten uint32
	for i := uint32(0); i < iovsCount; i++ {
		iov := iovs + i*8
		offset, ok := mod.Memory().ReadUint32Le(ctx, iov)
		if !ok {
			return ErrnoFault
		}
		// Note: emscripten has been known to write zero length iovec. However,
		// it is not common in other compilers, so we don't optimize for it.
		l, ok := mod.Memory().ReadUint32Le(ctx, iov+4)
		if !ok {
			return ErrnoFault
		}

		var n int
		if writer == io.Discard { // special-case default
			n = int(l)
		} else {
			b, ok := mod.Memory().Read(ctx, offset, l)
			if !ok {
				return ErrnoFault
			}
			n, err = writer.Write(b)
			if err != nil {
				return ErrnoIo
			}
		}
		nwritten += uint32(n)
	}
	if !mod.Memory().WriteUint32Le(ctx, resultSize, nwritten) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// pathCreateDirectory is the WASI function named functionPathCreateDirectory
// which creates a directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_create_directoryfd-fd-path-string---errno
var pathCreateDirectory = stubFunction(
	functionPathCreateDirectory,
	[]wasm.ValueType{i32, i32, i32},
	[]string{"fd", "path", "path_len"},
)

// pathFilestatGet is the WASI function named functionPathFilestatGet which
// returns the stat attributes of a file or directory.
//
// # Parameters
//
//   - fd: file descriptor of the folder to look in for the path
//   - flags: flags determining the method of how paths are resolved
//   - path: path under fd to get the filestat attributes data for
//   - path_len: length of the path that was given
//   - resultFilestat: offset to write the result filestat data
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoNotdir: `fd` points to a file not a directory
//   - ErrnoIo: could not stat `fd` on filesystem
//   - ErrnoInval: the path contained "../"
//   - ErrnoNametoolong: `path` + `path_len` is out of memory
//   - ErrnoFault: `resultFilestat` points to an offset out of memory
//   - ErrnoNoent: could not find the path
//
// The rest of this implementation matches that of fdFilestatGet, so is not
// repeated here.
//
// Note: This is similar to `fstatat` in POSIX.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_getfd-fd-flags-lookupflags-path-string---errno-filestat
// and https://linux.die.net/man/2/fstatat
var pathFilestatGet = &wasm.HostFunc{
	ExportNames: []string{functionPathFilestatGet},
	Name:        functionPathFilestatGet,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32, i32},
	ParamNames:  []string{"fd", "flags", "path", "path_len", "result.buf"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(pathFilestatGetFn),
	},
}

func pathFilestatGetFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	fsc := sysCtx.FS(ctx)

	fd := uint32(params[0])

	// TODO: implement flags?
	// flags := uint32(params[1])

	pathOffset := uint32(params[2])
	pathLen := uint32(params[3])
	resultBuf := uint32(params[4])

	// open_at isn't supported in fs.FS, so we check the path can't escape,
	// then join it with its parent
	b, ok := mod.Memory().Read(ctx, pathOffset, pathLen)
	if !ok {
		return ErrnoNametoolong
	}
	pathName := string(b)

	if dir, ok := fsc.OpenedFile(ctx, fd); !ok {
		return ErrnoBadf
	} else if dir.File == nil { // root
	} else if _, ok := dir.File.(fs.ReadDirFile); !ok {
		return ErrnoNotdir
	} else {
		pathName = path.Join(dir.Path, pathName)
	}

	// Sadly, we need to open the file to stat it.
	pathFd, errnoResult := openFile(ctx, fsc, pathName)
	if errnoResult != ErrnoSuccess {
		return errnoResult
	}

	// Close it when the function returns.
	defer fsc.CloseFile(ctx, pathFd)
	return fdFilestatGetFunc(ctx, mod, pathFd, resultBuf)
}

// pathFilestatSetTimes is the WASI function named functionPathFilestatSetTimes
// which adjusts the timestamps of a file or directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_set_timesfd-fd-flags-lookupflags-path-string-atim-timestamp-mtim-timestamp-fst_flags-fstflags---errno
var pathFilestatSetTimes = stubFunction(
	functionPathFilestatSetTimes,
	[]wasm.ValueType{i32, i32, i32, i32, i64, i64, i32},
	[]string{"fd", "flags", "path", "path_len", "atim", "mtim", "fst_flags"},
)

// pathLink is the WASI function named functionPathLink which adjusts the
// timestamps of a file or directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#path_link
var pathLink = stubFunction(
	functionPathLink,
	[]wasm.ValueType{i32, i32, i32, i32, i32, i32, i32},
	[]string{"old_fd", "old_flags", "old_path", "old_path_len", "new_fd", "new_path", "new_path_len"},
)

// pathOpen is the WASI function named functionPathOpen which opens a file or
// directory. This returns ErrnoBadf if the fd is invalid.
//
// # Parameters
//
//   - fd: file descriptor of a directory that `path` is relative to
//   - dirflags: flags to indicate how to resolve `path`
//   - path: offset in api.Memory to read the path string from
//   - pathLen: length of `path`
//   - oFlags: open flags to indicate the method by which to open the file
//   - fsRightsBase: ignored as rights were removed from WASI.
//   - fsRightsInheriting: ignored as rights were removed from WASI.
//     created file descriptor for `path`
//   - fdFlags: file descriptor flags
//   - resultOpenedFd: offset in api.Memory to write the newly created file
//     descriptor to.
//   - The result FD value is guaranteed to be less than 2**31
//
// Result (Errno)
//
// The return value is ErrnoSuccess except the following error conditions:
//   - ErrnoBadf: `fd` is invalid
//   - ErrnoFault: `resultOpenedFd` points to an offset out of memory
//   - ErrnoNoent: `path` does not exist.
//   - ErrnoExist: `path` exists, while `oFlags` requires that it must not.
//   - ErrnoNotdir: `path` is not a directory, while `oFlags` requires it.
//   - ErrnoIo: a file system error
//
// For example, this function needs to first read `path` to determine the file
// to open. If parameters `path` = 1, `pathLen` = 6, and the path is "wazero",
// pathOpen reads the path from api.Memory:
//
//	                pathLen
//	            +------------------------+
//	            |                        |
//	[]byte{ ?, 'w', 'a', 'z', 'e', 'r', 'o', ?... }
//	     path --^
//
// Then, if parameters resultOpenedFd = 8, and this function opened a new file
// descriptor 5 with the given flags, this function writes the below to
// api.Memory:
//
//	                  uint32le
//	                 +--------+
//	                 |        |
//	[]byte{ 0..6, ?, 5, 0, 0, 0, ?}
//	resultOpenedFd --^
//
// # Notes
//   - This is similar to `openat` in POSIX. https://linux.die.net/man/3/openat
//   - The returned file descriptor is not guaranteed to be the lowest-number
//
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_open
var pathOpen = &wasm.HostFunc{
	ExportNames: []string{functionPathOpen},
	Name:        functionPathOpen,
	ParamTypes:  []api.ValueType{i32, i32, i32, i32, i32, i64, i64, i32, i32},
	ParamNames:  []string{"fd", "dirflags", "path", "path_len", "oflags", "fs_rights_base", "fs_rights_inheriting", "fdflags", "result.opened_fd"},
	ResultTypes: []api.ValueType{i32},
	Code: &wasm.Code{
		IsHostFunction: true,
		GoFunc:         wasiFunc(pathOpenFn),
	},
}

func pathOpenFn(ctx context.Context, mod api.Module, params []uint64) Errno {
	sysCtx := mod.(*wasm.CallContext).Sys
	fsc := sysCtx.FS(ctx)

	fd := uint32(params[0])
	_ /* dirflags */ = uint32(params[1])
	path := uint32(params[2])
	pathLen := uint32(params[3])
	_ /* oflags */ = uint32(params[4])
	// rights aren't used
	_, _ = params[5], params[6]
	_ /* fdflags */ = uint32(params[7])
	resultOpenedFd := uint32(params[8])

	if _, ok := fsc.OpenedFile(ctx, fd); !ok {
		return ErrnoBadf
	}

	b, ok := mod.Memory().Read(ctx, path, pathLen)
	if !ok {
		return ErrnoFault
	}

	newFD, errnoResult := openFile(ctx, fsc, string(b))
	if errnoResult != ErrnoSuccess {
		return errnoResult
	}

	if !mod.Memory().WriteUint32Le(ctx, resultOpenedFd, newFD) {
		_ = fsc.CloseFile(ctx, newFD)
		return ErrnoFault
	}
	return ErrnoSuccess
}

// pathReadlink is the WASI function named functionPathReadlink that reads the
// contents of a symbolic link.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_readlinkfd-fd-path-string-buf-pointeru8-buf_len-size---errno-size
var pathReadlink = stubFunction(
	functionPathReadlink,
	[]wasm.ValueType{i32, i32, i32, i32, i32, i32},
	[]string{"fd", "path", "path_len", "buf", "buf_len", "result.bufused"},
)

// pathRemoveDirectory is the WASI function named functionPathRemoveDirectory
// which removes a directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_remove_directoryfd-fd-path-string---errno
var pathRemoveDirectory = stubFunction(
	functionPathRemoveDirectory,
	[]wasm.ValueType{i32, i32, i32},
	[]string{"fd", "path", "path_len"},
)

// pathRename is the WASI function named functionPathRename which renames a
// file or directory.
var pathRename = stubFunction(
	functionPathRename,
	[]wasm.ValueType{i32, i32, i32, i32, i32, i32},
	[]string{"fd", "old_path", "old_path_len", "new_fd", "new_path", "new_path_len"},
)

// pathSymlink is the WASI function named functionPathSymlink which creates a
// symbolic link.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#path_symlink
var pathSymlink = stubFunction(
	functionPathSymlink,
	[]wasm.ValueType{i32, i32, i32, i32, i32},
	[]string{"old_path", "old_path_len", "fd", "new_path", "new_path_len"},
)

// pathUnlinkFile is the WASI function named functionPathUnlinkFile which
// unlinks a file.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_unlink_filefd-fd-path-string---errno
var pathUnlinkFile = stubFunction(
	functionPathUnlinkFile,
	[]wasm.ValueType{i32, i32, i32},
	[]string{"fd", "path", "path_len"},
)

// openFile attempts to open the file at the given path. Errors coerce to WASI
// Errno, returned as a slice to avoid allocation per-error.
//
// Note: Coercion isn't centralized in internalsys.FSContext because ABI use
// different error codes. For example, wasi-filesystem and GOOS=js don't map to
// these Errno.
func openFile(ctx context.Context, fsc *internalsys.FSContext, name string) (fd uint32, errno Errno) {
	newFD, err := fsc.OpenFile(ctx, name)
	if err == nil {
		fd = newFD
		errno = ErrnoSuccess
		return
	}
	// handle all the cases of FS.Open or internal to FSContext.OpenFile
	switch {
	case errors.Is(err, fs.ErrInvalid):
		errno = ErrnoInval
	case errors.Is(err, fs.ErrNotExist):
		// fs.FS is allowed to return this instead of ErrInvalid on an invalid path
		errno = ErrnoNoent
	case errors.Is(err, fs.ErrExist):
		errno = ErrnoExist
	case errors.Is(err, syscall.EBADF):
		// fsc.OpenFile currently returns this on out of file descriptors
		errno = ErrnoBadf
	default:
		errno = ErrnoIo
	}
	return
}
