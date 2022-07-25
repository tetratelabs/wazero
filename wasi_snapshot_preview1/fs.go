package wasi_snapshot_preview1

import (
	"context"
	"errors"
	"io"
	"io/fs"

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
var fdClose = wasm.NewGoFunc(
	functionFdClose, functionFdClose,
	[]string{"fd"},
	func(ctx context.Context, mod api.Module, fd uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		if ok := sysCtx.FS(ctx).CloseFile(ctx, fd); !ok {
			return ErrnoBadf
		}

		return ErrnoSuccess
	},
)

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
//   - fs_filetype 1 byte, to indicate the file type
//   - fs_flags 2 bytes, to indicate the file descriptor flag
//   - 5 pad bytes
//   - fs_right_base 8 bytes, to indicate the current rights of the fd
//   - fs_right_inheriting 8 bytes, to indicate the maximum rights of the fd
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
var fdFdstatGet = wasm.NewGoFunc(
	functionFdFdstatGet, functionFdFdstatGet,
	[]string{"fd", "result.stat"},
	func(ctx context.Context, mod api.Module, fd uint32, resultStat uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		if _, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
			return ErrnoBadf
		}
		// TODO: actually write the fdstat!
		return ErrnoSuccess
	},
)

// fdFdstatSetFlags is the WASI function named functionFdFdstatSetFlags which
// adjusts the flags associated with a file descriptor.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_flagsfd-fd-flags-fdflags---errnoand is stubbed for GrainLang per #271
var fdFdstatSetFlags = stubFunction(
	functionFdFdstatSetFlags,
	[]wasm.ValueType{i32, i32},
	[]string{"fd", "flags"},
)

// fdFdstatSetRights is the WASI function named functionFdFdstatSetRights which
// adjusts the rights associated with a file descriptor.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_rightsfd-fd-fs_rights_base-rights-fs_rights_inheriting-rights---errno
//
// Note: This will never be implemented per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
var fdFdstatSetRights = stubFunction(
	functionFdFdstatSetRights,
	[]wasm.ValueType{i32, i64, i64},
	[]string{"fd", "fs_rights_base", "fs_rights_inheriting"},
)

// fdFilestatGet is the WASI function named functionFdFilestatGet which returns
// the attributes of an open file.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_getfd-fd---errno-filestat
var fdFilestatGet = stubFunction(
	functionFdFilestatGet,
	[]wasm.ValueType{i32, i32},
	[]string{"fd", "result.buf"},
)

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
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_preadfd-fd-iovs-iovec_array-offset-filesize---errno-size
var fdPread = stubFunction(
	functionFdPread,
	[]wasm.ValueType{i32, i32, i32, i64, i32},
	[]string{"fd", "iovs", "iovs_len", "offset", "result.nread"},
)

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
var fdPrestatGet = wasm.NewGoFunc(
	functionFdPrestatGet, functionFdPrestatGet,
	[]string{"fd", "result.prestat"},
	func(ctx context.Context, mod api.Module, fd uint32, resultPrestat uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
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
	},
)

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
var fdPrestatDirName = wasm.NewGoFunc(
	functionFdPrestatDirName, functionFdPrestatDirName,
	[]string{"fd", "path", "path_len"},
	func(ctx context.Context, mod api.Module, fd uint32, pathPtr uint32, pathLen uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd)
		if !ok {
			return ErrnoBadf
		}

		// Some runtimes may have another semantics. See /RATIONALE.md
		if uint32(len(f.Path)) < pathLen {
			return ErrnoNametoolong
		}

		// TODO: fdPrestatDirName may have to return ErrnoNotdir if the type of the prestat data of `fd` is not a PrestatDir.
		if !mod.Memory().Write(ctx, pathPtr, []byte(f.Path)[:pathLen]) {
			return ErrnoFault
		}
		return ErrnoSuccess
	},
)

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
var fdRead = wasm.NewGoFunc(
	functionFdRead, functionFdRead,
	[]string{"fd", "iovs", "iovs_len", "result.size"},
	func(ctx context.Context, mod api.Module, fd, iovs, iovsCount, resultSize uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		reader := internalsys.FdReader(ctx, sysCtx, fd)
		if reader == nil {
			return ErrnoBadf
		}

		var nread uint32
		for i := uint32(0); i < iovsCount; i++ {
			iovPtr := iovs + i*8
			offset, ok := mod.Memory().ReadUint32Le(ctx, iovPtr)
			if !ok {
				return ErrnoFault
			}
			l, ok := mod.Memory().ReadUint32Le(ctx, iovPtr+4)
			if !ok {
				return ErrnoFault
			}
			b, ok := mod.Memory().Read(ctx, offset, l)
			if !ok {
				return ErrnoFault
			}
			n, err := reader.Read(b) // Note: n <= l
			nread += uint32(n)
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return ErrnoIo
			}
		}
		if !mod.Memory().WriteUint32Le(ctx, resultSize, nread) {
			return ErrnoFault
		}
		return ErrnoSuccess
	},
)

// fdReaddir is the WASI function named functionFdReaddir which reads directory
// entries from a directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_readdirfd-fd-buf-pointeru8-buf_len-size-cookie-dircookie---errno-size
var fdReaddir = stubFunction(
	functionFdReaddir,
	[]wasm.ValueType{i32, i32, i32, i64, i32},
	[]string{"fd", "buf", "buf_len", "cookie", "result.bufused"},
)

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
var fdSeek = wasm.NewGoFunc(
	functionFdSeek, functionFdSeek,
	[]string{"fd", "offset", "whence", "result.newoffset"},
	func(ctx context.Context, mod api.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
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

		if !mod.Memory().WriteUint32Le(ctx, resultNewoffset, uint32(newOffset)) {
			return ErrnoFault
		}

		return ErrnoSuccess
	},
)

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
var fdWrite = wasm.NewGoFunc(
	functionFdWrite, functionFdWrite,
	[]string{"fd", "iovs", "iovs_len", "result.size"},
	func(ctx context.Context, mod api.Module, fd, iovs, iovsCount, resultSize uint32) Errno {
		sysCtx := mod.(*wasm.CallContext).Sys
		writer := internalsys.FdWriter(ctx, sysCtx, fd)
		if writer == nil {
			return ErrnoBadf
		}

		var nwritten uint32
		for i := uint32(0); i < iovsCount; i++ {
			iovPtr := iovs + i*8
			offset, ok := mod.Memory().ReadUint32Le(ctx, iovPtr)
			if !ok {
				return ErrnoFault
			}
			// Note: emscripten has been known to write zero length iovec. However,
			// it is not common in other compilers, so we don't optimize for it.
			l, ok := mod.Memory().ReadUint32Le(ctx, iovPtr+4)
			if !ok {
				return ErrnoFault
			}
			b, ok := mod.Memory().Read(ctx, offset, l)
			if !ok {
				return ErrnoFault
			}
			n, err := writer.Write(b)
			if err != nil {
				return ErrnoIo
			}
			nwritten += uint32(n)
		}
		if !mod.Memory().WriteUint32Le(ctx, resultSize, nwritten) {
			return ErrnoFault
		}
		return ErrnoSuccess
	},
)

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
// returns the attributes of a file or directory.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_getfd-fd-flags-lookupflags-path-string---errno-filestat
var pathFilestatGet = stubFunction(
	functionPathFilestatGet,
	[]wasm.ValueType{i32, i32, i32, i32, i32},
	[]string{"fd", "flags", "path", "path_len", "result.buf"},
)

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
//   - fsRightsBase: rights of the newly created file descriptor for `path`
//   - fsRightsInheriting: rights of the file descriptors derived from the newly
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
//   - Rights will never be implemented per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
//
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_open
var pathOpen = wasm.NewGoFunc(
	functionPathOpen, functionPathOpen,
	[]string{"fd", "dirflags", "path", "path_len", "oflags", "fs_rights_base", "fs_rights_inheriting", "fdflags", "result.opened_fd"},
	func(ctx context.Context, mod api.Module, fd, dirflags, pathPtr, pathLen, oflags uint32, fsRightsBase,
		fsRightsInheriting uint64, fdflags, resultOpenedFd uint32) (errno Errno) {
		sysCtx := mod.(*wasm.CallContext).Sys
		fsc := sysCtx.FS(ctx)
		if _, ok := fsc.OpenedFile(ctx, fd); !ok {
			return ErrnoBadf
		}

		b, ok := mod.Memory().Read(ctx, pathPtr, pathLen)
		if !ok {
			return ErrnoFault
		}

		if newFD, err := fsc.OpenFile(ctx, string(b)); err != nil {
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return ErrnoNoent
			case errors.Is(err, fs.ErrExist):
				return ErrnoExist
			default:
				return ErrnoIo
			}
		} else if !mod.Memory().WriteUint32Le(ctx, resultOpenedFd, newFD) {
			_ = fsc.CloseFile(ctx, newFD)
			return ErrnoFault
		}
		return ErrnoSuccess
	},
)

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
