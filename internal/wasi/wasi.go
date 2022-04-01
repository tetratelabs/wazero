package internalwasi

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/tetratelabs/wazero/api"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
)

const (
	// FunctionStart is the name of the nullary function a module must export if it is a WASI Command Module.
	//
	// Note: When this is exported FunctionInitialize must not be.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	FunctionStart = "_start"

	// FunctionInitialize is the name of the nullary function a module must export if it is a WASI Reactor Module.
	//
	// Note: When this is exported FunctionStart must not be.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	FunctionInitialize = "_initialize"

	// FunctionArgsGet reads command-line argument data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	FunctionArgsGet = "args_get"

	// ImportArgsGet is the WebAssembly 1.0 (20191205) Text format import of FunctionArgsGet.
	ImportArgsGet = `(import "wasi_snapshot_preview1" "args_get"
    (func $wasi.args_get (param $argv i32) (param $argv_buf i32) (result (;errno;) i32)))`

	// FunctionArgsSizesGet returns command-line argument data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	FunctionArgsSizesGet = "args_sizes_get"

	// ImportArgsSizesGet is the WebAssembly 1.0 (20191205) Text format import of FunctionArgsSizesGet.
	ImportArgsSizesGet = `(import "wasi_snapshot_preview1" "args_sizes_get"
    (func $wasi.args_sizes_get (param $result.argc i32) (param $result.argv_buf_size i32) (result (;errno;) i32)))`

	// FunctionEnvironGet reads environment variable data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	FunctionEnvironGet = "environ_get"

	// ImportEnvironGet is the WebAssembly 1.0 (20191205) Text format import of FunctionEnvironGet.
	ImportEnvironGet = `(import "wasi_snapshot_preview1" "environ_get"
    (func $wasi.environ_get (param $environ i32) (param $environ_buf i32) (result (;errno;) i32)))`

	// FunctionEnvironSizesGet returns environment variable data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	FunctionEnvironSizesGet = "environ_sizes_get"

	// ImportEnvironSizesGet is the WebAssembly 1.0 (20191205) Text format import of FunctionEnvironSizesGet.
	ImportEnvironSizesGet = `(import "wasi_snapshot_preview1" "environ_sizes_get"
    (func $wasi.environ_sizes_get (param $result.environc i32) (param $result.environBufSize i32) (result (;errno;) i32)))`

	// FunctionClockResGet returns the resolution of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	FunctionClockResGet = "clock_res_get"

	// ImportClockResGet is the WebAssembly 1.0 (20191205) Text format import of FunctionClockResGet.
	ImportClockResGet = `(import "wasi_snapshot_preview1" "clock_res_get"
    (func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))`

	// FunctionClockTimeGet returns the time value of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	FunctionClockTimeGet = "clock_time_get"

	// ImportClockTimeGet is the WebAssembly 1.0 (20191205) Text format import of FunctionClockTimeGet.
	ImportClockTimeGet = `(import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))`

	// FunctionFdAdvise provides file advisory information on a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_advisefd-fd-offset-filesize-len-filesize-advice-advice---errno
	FunctionFdAdvise = "fd_advise"

	// ImportFdAdvise is the WebAssembly 1.0 (20191205) Text format import of FunctionFdAdvise.
	ImportFdAdvise = `(import "wasi_snapshot_preview1" "fd_advise"
    (func $wasi.fd_advise (param $fd i32) (param $offset i64) (param $len i64) (param $result.advice i32) (result (;errno;) i32)))`

	// FunctionFdAllocate forces the allocation of space in a file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_allocatefd-fd-offset-filesize-len-filesize---errno
	FunctionFdAllocate = "fd_allocate"

	// ImportFdAllocate is the WebAssembly 1.0 (20191205) Text format import of FunctionFdAllocate.
	ImportFdAllocate = `(import "wasi_snapshot_preview1" "fd_allocate"
    (func $wasi.fd_allocate (param $fd i32) (param $offset i64) (param $len i64) (result (;errno;) i32)))`

	// FunctionFdClose closes a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_close
	FunctionFdClose = "fd_close"

	// ImportFdClose is the WebAssembly 1.0 (20191205) Text format import of FunctionFdClose.
	ImportFdClose = `(import "wasi_snapshot_preview1" "fd_close"
    (func $wasi.fd_close (param $fd i32) (result (;errno;) i32)))`

	// FunctionFdDatasync synchronizes the data of a file to disk.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_close
	FunctionFdDatasync = "fd_datasync"

	// ImportFdDatasync is the WebAssembly 1.0 (20191205) Text format import of FunctionFdDatasync.
	ImportFdDatasync = `(import "wasi_snapshot_preview1" "fd_datasync"
    (func $wasi.fd_datasync (param $fd i32) (result (;errno;) i32)))`

	// FunctionFdFdstatGet gets the attributes of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_getfd-fd---errno-fdstat
	FunctionFdFdstatGet = "fd_fdstat_get"

	// ImportFdFdstatGet is the WebAssembly 1.0 (20191205) Text format import of FunctionFdFdstatGet.
	ImportFdFdstatGet = `(import "wasi_snapshot_preview1" "fd_fdstat_get"
    (func $wasi.fd_fdstat_get (param $fd i32) (param $result.stat i32) (result (;errno;) i32)))`

	// FunctionFdFdstatSetFlags adjusts the flags associated with a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_flagsfd-fd-flags-fdflags---errno
	FunctionFdFdstatSetFlags = "fd_fdstat_set_flags"

	// ImportFdFdstatSetFlags is the WebAssembly 1.0 (20191205) Text format import of FunctionFdFdstatSetFlags.
	ImportFdFdstatSetFlags = `(import "wasi_snapshot_preview1" "fd_fdstat_set_flags"
    (func $wasi.fd_fdstat_set_flags (param $fd i32) (param $flags i32) (result (;errno;) i32)))`

	// FunctionFdFdstatSetRights adjusts the rights associated with a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_rightsfd-fd-fs_rights_base-rights-fs_rights_inheriting-rights---errno
	FunctionFdFdstatSetRights = "fd_fdstat_set_rights"

	// ImportFdFdstatSetRights is the WebAssembly 1.0 (20191205) Text format import of FunctionFdFdstatSetRights.
	ImportFdFdstatSetRights = `(import "wasi_snapshot_preview1" "fd_fdstat_set_rights"
    (func $wasi.fd_fdstat_set_rights (param $fd i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (result (;errno;) i32)))`

	// FunctionFdFilestatGet returns the attributes of an open file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_getfd-fd---errno-filestat
	FunctionFdFilestatGet = "fd_filestat_get"

	// ImportFdFilestatGet is the WebAssembly 1.0 (20191205) Text format import of FunctionFdFilestatGet.
	ImportFdFilestatGet = `(import "wasi_snapshot_preview1" "fd_filestat_get"
    (func $wasi.fd_filestat_get (param $fd i32) (param $result.buf i32) (result (;errno;) i32)))`

	// FunctionFdFilestatSetSize adjusts the size of an open file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_set_sizefd-fd-size-filesize---errno
	FunctionFdFilestatSetSize = "fd_filestat_set_size"

	// ImportFdFilestatSetSize is the WebAssembly 1.0 (20191205) Text format import of FunctionFdFilestatSetSize.
	ImportFdFilestatSetSize = `(import "wasi_snapshot_preview1" "fd_filestat_set_size"
    (func $wasi.fd_filestat_set_size (param $fd i32) (param $size i64) (result (;errno;) i32)))`

	// FunctionFdFilestatSetTimes adjusts the times of an open file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_set_timesfd-fd-atim-timestamp-mtim-timestamp-fst_flags-fstflags---errno
	FunctionFdFilestatSetTimes = "fd_filestat_set_times"

	// ImportFdFilestatSetTimes is the WebAssembly 1.0 (20191205) Text format import of FunctionFdFilestatSetTimes.
	ImportFdFilestatSetTimes = `(import "wasi_snapshot_preview1" "fd_filestat_set_times"
    (func $wasi.fd_filestat_set_times (param $fd i32) (param $atim i64) (param $mtim i64) (param $fst_flags i32) (result (;errno;) i32)))`

	// FunctionFdPread reads from a file descriptor, without using and updating the file descriptor's offset.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_preadfd-fd-iovs-iovec_array-offset-filesize---errno-size
	FunctionFdPread = "fd_pread"

	// ImportFdPread is the WebAssembly 1.0 (20191205) Text format import of FunctionFdPread.
	ImportFdPread = `(import "wasi_snapshot_preview1" "fd_pread"
    (func $wasi.fd_pread (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $offset i64) (param $result.nread i32) (result (;errno;) i32)))`

	// FunctionFdPrestatGet returns the prestat data of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_get
	FunctionFdPrestatGet = "fd_prestat_get"

	// ImportFdPrestatGet is the WebAssembly 1.0 (20191205) Text format import of FunctionFdPrestatGet.
	ImportFdPrestatGet = `(import "wasi_snapshot_preview1" "fd_prestat_get"
    (func $wasi.fd_prestat_get (param $fd i32) (param $result.prestat i32) (result (;errno;) i32)))`

	// FunctionFdPrestatDirName returns the path of the pre-opened directory of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_dir_name
	FunctionFdPrestatDirName = "fd_prestat_dir_name"

	// ImportFdPrestatDirName is the WebAssembly 1.0 (20191205) Text format import of FunctionFdPrestatDirName.
	ImportFdPrestatDirName = `(import "wasi_snapshot_preview1" "fd_prestat_dir_name"
    (func $wasi.fd_prestat_dir_name (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// FunctionFdPwrite writes to a file descriptor, without using and updating the file descriptor's offset.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_pwritefd-fd-iovs-ciovec_array-offset-filesize---errno-size
	FunctionFdPwrite = "fd_pwrite"

	// ImportFdPwrite is the WebAssembly 1.0 (20191205) Text format import of FunctionFdPwrite.
	ImportFdPwrite = `(import "wasi_snapshot_preview1" "fd_pwrite"
    (func $wasi.fd_pwrite (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $offset i64) (param $result.nwritten i32) (result (;errno;) i32)))`

	// FunctionFdRead read bytes from a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_read
	FunctionFdRead = "fd_read"

	// ImportFdRead is the WebAssembly 1.0 (20191205) Text format import of FunctionFdRead.
	ImportFdRead = `(import "wasi_snapshot_preview1" "fd_read"
    (func $wasi.fd_read (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))`

	// FunctionFdReaddir reads directory entries from a directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_readdirfd-fd-buf-pointeru8-buf_len-size-cookie-dircookie---errno-size
	FunctionFdReaddir = "fd_readdir"

	// ImportFdReaddir is the WebAssembly 1.0 (20191205) Text format import of FunctionFdReaddir.
	ImportFdReaddir = `(import "wasi_snapshot_preview1" "fd_readdir"
    (func $wasi.fd_readdir (param $fd i32) (param $buf i32) (param $buf_len i32) (param $cookie i64) (param $result.bufused i32) (result (;errno;) i32)))`

	// FunctionFdRenumber atomically replaces a file descriptor by renumbering another file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_renumberfd-fd-to-fd---errno
	FunctionFdRenumber = "fd_renumber"

	// ImportFdRenumber is the WebAssembly 1.0 (20191205) Text format import of FunctionFdRenumber.
	ImportFdRenumber = `(import "wasi_snapshot_preview1" "fd_renumber"
    (func $wasi.fd_renumber (param $fd i32) (param $to i32) (result (;errno;) i32)))`

	// FunctionFdSeek moves the offset of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_seekfd-fd-offset-filedelta-whence-whence---errno-filesize
	FunctionFdSeek = "fd_seek"

	// ImportFdSeek is the WebAssembly 1.0 (20191205) Text format import of FunctionFdSeek.
	ImportFdSeek = `(import "wasi_snapshot_preview1" "fd_seek"
    (func $wasi.fd_seek (param $fd i32) (param $offset i64) (param $whence i32) (param $result.newoffset i32) (result (;errno;) i32)))`

	// FunctionFdSync synchronizes the data and metadata of a file to disk.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_syncfd-fd---errno
	FunctionFdSync = "fd_sync"

	// ImportFdSync is the WebAssembly 1.0 (20191205) Text format import of FunctionFdSync.
	ImportFdSync = `(import "wasi_snapshot_preview1" "fd_sync"
    (func $wasi.fd_sync (param $fd i32) (result (;errno;) i32)))`

	// FunctionFdTell returns the current offset of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_tellfd-fd---errno-filesize
	FunctionFdTell = "fd_tell"

	// ImportFdTell is the WebAssembly 1.0 (20191205) Text format import of FunctionFdTell.
	ImportFdTell = `(import "wasi_snapshot_preview1" "fd_tell"
    (func $wasi.fd_tell (param $fd i32) (param $result.offset i32) (result (;errno;) i32)))`

	// FunctionFdWrite write bytes to a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_write
	FunctionFdWrite = "fd_write"

	// ImportFdWrite is the WebAssembly 1.0 (20191205) Text format import of FunctionFdWrite.
	ImportFdWrite = `(import "wasi_snapshot_preview1" "fd_write"
    (func $wasi.fd_write (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))`

	// FunctionPathCreateDirectory creates a directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_create_directoryfd-fd-path-string---errno
	FunctionPathCreateDirectory = "path_create_directory"

	// ImportPathCreateDirectory is the WebAssembly 1.0 (20191205) Text format import of FunctionPathCreateDirectory.
	ImportPathCreateDirectory = `(import "wasi_snapshot_preview1" "path_create_directory"
    (func $wasi.path_create_directory (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// FunctionPathFilestatGet returns the attributes of a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_getfd-fd-flags-lookupflags-path-string---errno-filestat
	FunctionPathFilestatGet = "path_filestat_get"

	// ImportPathFilestatGet is the WebAssembly 1.0 (20191205) Text format import of FunctionPathFilestatGet.
	ImportPathFilestatGet = `(import "wasi_snapshot_preview1" "path_filestat_get"
    (func $wasi.path_filestat_get (param $fd i32) (param $flags i32) (param $path i32) (param $path_len i32) (param $result.buf i32) (result (;errno;) i32)))`

	// FunctionPathFilestatSetTimes adjusts the timestamps of a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_set_timesfd-fd-flags-lookupflags-path-string-atim-timestamp-mtim-timestamp-fst_flags-fstflags---errno
	FunctionPathFilestatSetTimes = "path_filestat_set_times"

	// ImportPathFilestatSetTimes is the WebAssembly 1.0 (20191205) Text format import of FunctionPathFilestatSetTimes.
	ImportPathFilestatSetTimes = `(import "wasi_snapshot_preview1" "path_filestat_set_times"
    (func $wasi.path_filestat_set_times (param $fd i32) (param $flags i32) (param $path i32) (param $path_len i32) (param $atim i64) (param $mtim i64) (param $fst_flags i32) (result (;errno;) i32)))`

	// FunctionPathLink adjusts the timestamps of a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#path_link
	FunctionPathLink = "path_link"

	// ImportPathLink is the WebAssembly 1.0 (20191205) Text format import of FunctionPathLink.
	ImportPathLink = `(import "wasi_snapshot_preview1" "path_link"
    (func $wasi.path_link (param $old_fd i32) (param $old_flags i32) (param $old_path i32) (param $old_path_len i32) (param $new_fd i32) (param $new_path i32) (param $new_path_len i32) (result (;errno;) i32)))`

	// FunctionPathOpen opens a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_openfd-fd-dirflags-lookupflags-path-string-oflags-oflags-fs_rights_base-rights-fs_rights_inheriting-rights-fdflags-fdflags---errno-fd
	FunctionPathOpen = "path_open"

	// ImportPathOpen is the WebAssembly 1.0 (20191205) Text format import of FunctionPathOpen.
	ImportPathOpen = `(import "wasi_snapshot_preview1" "path_open"
    (func $wasi.path_open (param $fd i32) (param $dirflags i32) (param $path i32) (param $path_len i32) (param $oflags i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (param $fdflags i32) (param $result.opened_fd i32) (result (;errno;) i32)))`

	// FunctionPathReadlink reads the contents of a symbolic link.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_readlinkfd-fd-path-string-buf-pointeru8-buf_len-size---errno-size
	FunctionPathReadlink = "path_readlink"

	// ImportPathReadlink is the WebAssembly 1.0 (20191205) Text format import of FunctionPathReadlink.
	ImportPathReadlink = `(import "wasi_snapshot_preview1" "path_readlink"
    (func $wasi.path_readlink (param $fd i32) (param $path i32) (param $path_len i32) (param $buf i32) (param $buf_len i32) (param $result.bufused i32) (result (;errno;) i32)))`

	// FunctionPathRemoveDirectory removes a directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_remove_directoryfd-fd-path-string---errno
	FunctionPathRemoveDirectory = "path_remove_directory"

	// ImportPathRemoveDirectory is the WebAssembly 1.0 (20191205) Text format import of FunctionPathRemoveDirectory.
	ImportPathRemoveDirectory = `(import "wasi_snapshot_preview1" "path_remove_directory"
    (func $wasi.path_remove_directory (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// FunctionPathRename renames a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_renamefd-fd-old_path-string-new_fd-fd-new_path-string---errno
	FunctionPathRename = "path_rename"

	// ImportPathRename is the WebAssembly 1.0 (20191205) Text format import of FunctionPathRename.
	ImportPathRename = `(import "wasi_snapshot_preview1" "path_rename"
    (func $wasi.path_rename (param $fd i32) (param $old_path i32) (param $old_path_len i32) (param $new_fd i32) (param $new_path i32) (param $new_path_len i32) (result (;errno;) i32)))`

	// FunctionPathSymlink creates a symbolic link.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#path_symlink
	FunctionPathSymlink = "path_symlink"

	// ImportPathSymlink is the WebAssembly 1.0 (20191205) Text format import of FunctionPathSymlink.
	ImportPathSymlink = `(import "wasi_snapshot_preview1" "path_symlink"
    (func $wasi.path_symlink (param $old_path i32) (param $old_path_len i32) (param $fd i32) (param $new_path i32) (param $new_path_len i32) (result (;errno;) i32)))`

	// FunctionPathUnlinkFile unlinks a file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_unlink_filefd-fd-path-string---errno
	FunctionPathUnlinkFile = "path_unlink_file"

	// ImportPathUnlinkFile is the WebAssembly 1.0 (20191205) Text format import of FunctionPathUnlinkFile.
	ImportPathUnlinkFile = `(import "wasi_snapshot_preview1" "path_unlink_file"
    (func $wasi.path_unlink_file (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// FunctionPollOneoff unlinks a file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-poll_oneoffin-constpointersubscription-out-pointerevent-nsubscriptions-size---errno-size
	FunctionPollOneoff = "poll_oneoff"

	// ImportPollOneoff is the WebAssembly 1.0 (20191205) Text format import of FunctionPollOneoff.
	ImportPollOneoff = `(import "wasi_snapshot_preview1" "poll_oneoff"
    (func $wasi.poll_oneoff (param $in i32) (param $out i32) (param $nsubscriptions i32) (param $result.nevents i32) (result (;errno;) i32)))`

	// FunctionProcExit terminates the execution of the module with an exit code.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
	FunctionProcExit = "proc_exit"

	// ImportProcExit is the WebAssembly 1.0 (20191205) Text format import of FunctionProcExit.
	//
	// See ImportProcExit
	// See SnapshotPreview1.ProcExit
	// See FunctionProcExit
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#proc_exit
	ImportProcExit = `(import "wasi_snapshot_preview1" "proc_exit"
    (func $wasi.proc_exit (param $rval i32)))`

	// FunctionProcRaise sends a signal to the process of the calling thread.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-proc_raisesig-signal---errno
	FunctionProcRaise = "proc_raise"

	// ImportProcRaise is the WebAssembly 1.0 (20191205) Text format import of FunctionProcRaise.
	ImportProcRaise = `(import "wasi_snapshot_preview1" "proc_raise"
    (func $wasi.proc_raise (param $sig i32) (result (;errno;) i32)))`

	// FunctionSchedYield temporarily yields execution of the calling thread.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sched_yield---errno
	FunctionSchedYield = "sched_yield"

	// ImportSchedYield is the WebAssembly 1.0 (20191205) Text format import of FunctionSchedYield.
	ImportSchedYield = `(import "wasi_snapshot_preview1" "sched_yield"
    (func $wasi.sched_yield (result (;errno;) i32)))`

	// FunctionRandomGet writes random data in buffer.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-buf_len-size---errno
	FunctionRandomGet = "random_get"

	// ImportRandomGet is the WebAssembly 1.0 (20191205) Text format import of FunctionRandomGet.
	ImportRandomGet = `(import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))`

	// FunctionSockRecv receives a message from a socket.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_recvfd-fd-ri_data-iovec_array-ri_flags-riflags---errno-size-roflags
	FunctionSockRecv = "sock_recv"

	// ImportSockRecv is the WebAssembly 1.0 (20191205) Text format import of FunctionSockRecv.
	ImportSockRecv = `(import "wasi_snapshot_preview1" "sock_recv"
    (func $wasi.sock_recv (param $fd i32) (param $ri_data i32) (param $ri_data_count i32) (param $ri_flags i32) (param $result.ro_datalen i32) (param $result.ro_flags i32) (result (;errno;) i32)))`

	// FunctionSockSend sends a message on a socket.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_sendfd-fd-si_data-ciovec_array-si_flags-siflags---errno-size
	FunctionSockSend = "sock_send"

	// ImportSockSend is the WebAssembly 1.0 (20191205) Text format import of FunctionSockSend.
	ImportSockSend = `(import "wasi_snapshot_preview1" "sock_send"
    (func $wasi.sock_send (param $fd i32) (param $si_data i32) (param $si_data_count i32) (param $si_flags i32) (param $result.so_datalen i32) (result (;errno;) i32)))`

	// FunctionSockShutdown shuts down socket send and receive channels.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_shutdownfd-fd-how-sdflags---errno
	FunctionSockShutdown = "sock_shutdown"

	// ImportSockShutdown is the WebAssembly 1.0 (20191205) Text format import of FunctionSockShutdown.
	ImportSockShutdown = `(import "wasi_snapshot_preview1" "sock_shutdown"
    (func $wasi.sock_shutdown (param $fd i32) (param $how i32) (result (;errno;) i32)))`
)

// SnapshotPreview1 includes all host functions to export for WASI version "wasi_snapshot_preview1".
//
// ## Translation notes
// ### String
// WebAssembly 1.0 (20191205) has no string type, so any string input parameter expands to two uint32 parameters: offset
// and length.
//
// ### iovec_array
// `iovec_array` is encoded as two uin32le values (i32): offset and count.
//
// ### Result
// Each result besides wasi.Errno is always an uint32 parameter. WebAssembly 1.0 (20191205) can have up to one result,
// which is already used by wasi.Errno. This forces other results to be parameters. A result parameter is a memory
// offset to write the result to. As memory offsets are uint32, each parameter representing a result is uint32.
//
// ### Errno
// The WASI specification is sometimes ambiguous resulting in some runtimes interpreting the same function ways.
// wasi.Errno mappings are not defined in WASI, yet, so these mappings are best efforts by maintainers. When in doubt
// about portability, first look at internal/wasi/RATIONALE.md and if needed an issue on
// https://github.com/WebAssembly/WASI/issues
//
// ## Memory
// In WebAssembly 1.0 (20191205), there may be up to one Memory per store, which means api.Memory is always the
// wasm.Store Memories index zero: `store.Memories[0].Buffer`
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
// See https://github.com/WebAssembly/WASI/issues/215
// See https://wwa.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0.
type SnapshotPreview1 interface {
	// ArgsGet is the WASI function that reads command-line argument data (WithArgs).
	//
	// There are two parameters. Both are offsets in api.Module Memory. If either are invalid due to
	// memory constraints, this returns ErrnoFault.
	//
	// * argv - is the offset to begin writing argument offsets in uint32 little-endian encoding.
	//   * ArgsSizesGet result argc * 4 bytes are written to this offset
	// * argvBuf - is the offset to write the null terminated arguments to m.Memory
	//   * ArgsSizesGet result argv_buf_size bytes are written to this offset
	//
	// For example, if ArgsSizesGet wrote argc=2 and argvBufSize=5 for arguments: "a" and "bc"
	//    parameters argv=7 and argvBuf=1, this function writes the below to `m.Memory`:
	//
	//               argvBufSize          uint32le    uint32le
	//            +----------------+     +--------+  +--------+
	//            |                |     |        |  |        |
	// []byte{?, 'a', 0, 'b', 'c', 0, ?, 1, 0, 0, 0, 3, 0, 0, 0, ?}
	//  argvBuf --^                      ^           ^
	//                            argv --|           |
	//          offset that begins "a" --+           |
	//                     offset that begins "bc" --+
	//
	// Note: ImportArgsGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See ArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsGet(m api.Module, argv, argvBuf uint32) api.Errno

	// ArgsSizesGet is the WASI function named FunctionArgsSizesGet that reads command-line argument data (WithArgs)
	// sizes.
	//
	// There are two result parameters: these are offsets in the api.Module Memory to write
	// corresponding sizes in uint32 little-endian encoding. If either are invalid due to memory constraints, this
	// returns ErrnoFault.
	//
	// * resultArgc - is the offset to write the argument count to m.Memory
	// * resultArgvBufSize - is the offset to write the null-terminated argument length to m.Memory
	//
	// For example, if WithArgs are []string{"a","bc"} and
	//    parameters resultArgc=1 and resultArgvBufSize=6, this function writes the below to `m.Memory`:
	//
	//                   uint32le       uint32le
	//                  +--------+     +--------+
	//                  |        |     |        |
	//        []byte{?, 2, 0, 0, 0, ?, 5, 0, 0, 0, ?}
	//     resultArgc --^              ^
	//         2 args --+              |
	//             resultArgvBufSize --|
	//   len([]byte{'a',0,'b',c',0}) --+
	//
	// Note: ImportArgsSizesGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See ArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsSizesGet(m api.Module, resultArgc, resultArgvBufSize uint32) api.Errno

	// EnvironGet is the WASI function named FunctionEnvironGet that reads environment variables. (WithEnviron)
	//
	// There are two parameters. Both are offsets in api.Module Memory. If either are invalid due to
	// memory constraints, this returns ErrnoFault.
	//
	// * environ - is the offset to begin writing environment variables offsets in uint32 little-endian encoding.
	//   * EnvironSizesGet result environc * 4 bytes are written to this offset
	// * environBuf - is the offset to write the environment variables to m.Memory
	//   * the format is the same as os.Environ, null terminated "key=val" entries
	//   * EnvironSizesGet result environBufSize bytes are written to this offset
	//
	// For example, if EnvironSizesGet wrote environc=2 and environBufSize=9 for environment variables: "a=b", "b=cd"
	//   and parameters environ=11 and environBuf=1, this function writes the below to `m.Memory`:
	//
	//                           environBufSize                 uint32le    uint32le
	//              +------------------------------------+     +--------+  +--------+
	//              |                                    |     |        |  |        |
	//   []byte{?, 'a', '=', 'b', 0, 'b', '=', 'c', 'd', 0, ?, 1, 0, 0, 0, 5, 0, 0, 0, ?}
	// environBuf --^                                          ^           ^
	//                              environ offset for "a=b" --+           |
	//                                         environ offset for "b=cd" --+
	//
	// Note: ImportEnvironGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See EnvironSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	EnvironGet(m api.Module, environ, environBuf uint32) api.Errno

	// EnvironSizesGet is the WASI function named FunctionEnvironSizesGet that reads environment variable
	// (WithEnviron) sizes.
	//
	// There are two result parameters: these are offsets in the wasi.Module Memory to write
	// corresponding sizes in uint32 little-endian encoding. If either are invalid due to memory constraints, this
	// returns ErrnoFault.
	//
	// * resultEnvironc - is the offset to write the environment variable count to m.Memory
	// * resultEnvironBufSize - is the offset to write the null-terminated environment variable length to m.Memory
	//
	// For example, if WithEnviron is []string{"a=b","b=cd"} and
	//    parameters resultEnvironc=1 and resultEnvironBufSize=6, this function writes the below to `m.Memory`:
	//
	//                   uint32le       uint32le
	//                  +--------+     +--------+
	//                  |        |     |        |
	//        []byte{?, 2, 0, 0, 0, ?, 9, 0, 0, 0, ?}
	// resultEnvironc --^              ^
	//    2 variables --+              |
	//          resultEnvironBufSize --|
	//    len([]byte{'a','=','b',0,    |
	//           'b','=','c','d',0}) --+
	//
	// Note: ImportEnvironGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See EnvironGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	EnvironSizesGet(m api.Module, resultEnvironc, resultEnvironBufSize uint32) api.Errno

	// ClockResGet is the WASI function named FunctionClockResGet and is stubbed for GrainLang per #271
	ClockResGet(m api.Module, id uint32, resultResolution uint32) api.Errno

	// ClockTimeGet is the WASI function named FunctionClockTimeGet that returns the time value of a clock (time.Now).
	//
	// * id - The clock id for which to return the time.
	// * precision - The maximum lag (exclusive) that the returned time value may have, compared to its actual value.
	// * resultTimestamp - the offset to write the timestamp to m.Memory
	//   * the timestamp is epoch nanoseconds encoded as a uint64 little-endian encoding.
	//
	// For example, if time.Now returned exactly midnight UTC 2022-01-01 (1640995200000000000), and
	//   parameters resultTimestamp=1, this function writes the below to `m.Memory`:
	//
	//                                      uint64le
	//                    +------------------------------------------+
	//                    |                                          |
	//          []byte{?, 0x0, 0x0, 0x1f, 0xa6, 0x70, 0xfc, 0xc5, 0x16, ?}
	//  resultTimestamp --^
	//
	// Note: ImportClockTimeGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `clock_gettime` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	// See https://linux.die.net/man/3/clock_gettime
	ClockTimeGet(m api.Module, id uint32, precision uint64, resultTimestamp uint32) api.Errno

	// FdAdvise is the WASI function named FunctionFdAdvise and is stubbed for GrainLang per #271
	FdAdvise(m api.Module, fd uint32, offset, len uint64, resultAdvice uint32) api.Errno

	// FdAllocate is the WASI function named FunctionFdAllocate and is stubbed for GrainLang per #271
	FdAllocate(m api.Module, fd uint32, offset, len uint64, resultAdvice uint32) api.Errno

	// FdClose is the WASI function to close a file descriptor. This returns ErrnoBadf if the fd is invalid.
	//
	// * fd - the file descriptor to close
	//
	// Note: ImportFdClose shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `close` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_close
	// See https://linux.die.net/man/3/close
	FdClose(m api.Module, fd uint32) api.Errno

	// FdDatasync is the WASI function named FunctionFdDatasync and is stubbed for GrainLang per #271
	FdDatasync(m api.Module, fd uint32) api.Errno

	// FdFdstatGet is the WASI function to return the attributes of a file descriptor.
	//
	// * fd - the file descriptor to get the fdstat attributes data
	// * resultFdstat - the offset to write the result fdstat data
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `resultFdstat` contains an invalid offset due to the memory constraint
	//
	// fdstat byte layout is 24-byte size, which as the following elements in order
	// * fs_filetype 1 byte, to indicate the file type
	// * fs_flags 2 bytes, to indicate the file descriptor flag
	// * 5 pad bytes
	// * fs_right_base 8 bytes, to indicate the current rights of the fd
	// * fs_right_inheriting 8 bytes, to indicate the maximum rights of the fd
	//
	// For example, with a file corresponding with `fd` was a directory (=3) opened with `fd_read` right (=1) and no fs_flags (=0),
	//    parameter resultFdstat=1, this function writes the below to `m.Memory`:
	//
	//                   uint16le   padding            uint64le                uint64le
	//          uint8 --+  +--+  +-----------+  +--------------------+  +--------------------+
	//                  |  |  |  |           |  |                    |  |                    |
	//        []byte{?, 3, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0}
	//   resultFdstat --^  ^-- fs_flags         ^-- fs_right_base       ^-- fs_right_inheriting
	//                  |
	//                  +-- fs_filetype
	//
	// Note: ImportFdFdstatGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: FdFdstatGet returns similar flags to `fsync(fd, F_GETFL)` in POSIX, as well as additional fields.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fdstat
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_fdstat_get
	// See https://linux.die.net/man/3/fsync
	FdFdstatGet(m api.Module, fd, resultFdstat uint32) api.Errno

	// FdFdstatSetFlags is the WASI function named FunctionFdFdstatSetFlags and is stubbed for GrainLang per #271
	FdFdstatSetFlags(m api.Module, fd uint32, flags uint32) api.Errno

	// FdFdstatSetRights is the WASI function named FunctionFdFdstatSetRights and is stubbed for GrainLang per #271
	FdFdstatSetRights(m api.Module, fd uint32, fsRightsBase, fsRightsInheriting uint64) api.Errno

	// FdFilestatGet is the WASI function named FunctionFdFilestatGet
	FdFilestatGet(m api.Module, fd uint32, resultBuf uint32) api.Errno

	// FdFilestatSetSize is the WASI function named FunctionFdFilestatSetSize
	FdFilestatSetSize(m api.Module, fd uint32, size uint64) api.Errno

	// FdFilestatSetTimes is the WASI function named FunctionFdFilestatSetTimes
	FdFilestatSetTimes(m api.Module, fd uint32, atim, mtim uint64, fstFlags uint32) api.Errno

	// FdPread is the WASI function named FunctionFdPread
	FdPread(m api.Module, fd, iovs uint32, offset uint64, resultNread uint32) api.Errno

	// FdPrestatGet is the WASI function to return the prestat data of a file descriptor.
	//
	// * fd - the file descriptor to get the prestat
	// * resultPrestat - the offset to write the result prestat data
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid or the `fd` is not a pre-opened directory.
	// * wasi.ErrnoFault - if `resultPrestat` is an invalid offset due to the memory constraint
	//
	// prestat byte layout is 8 bytes, beginning with an 8-bit tag and 3 pad bytes. The only valid tag is `prestat_dir`,
	// which is tag zero. This simplifies the byte layout to 4 empty bytes followed by the uint32le encoded path length.
	//
	// For example, the directory name corresponding with `fd` was "/tmp" and
	//    parameter resultPrestat=1, this function writes the below to `m.Memory`:
	//
	//                     padding   uint32le
	//          uint8 --+  +-----+  +--------+
	//                  |  |     |  |        |
	//        []byte{?, 0, 0, 0, 0, 4, 0, 0, 0, ?}
	//  resultPrestat --^           ^
	//            tag --+           |
	//                              +-- size in bytes of the string "/tmp"
	//
	// Note: ImportFdPrestatGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See FdPrestatDirName
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#prestat
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_prestat_get
	FdPrestatGet(m api.Module, fd uint32, resultPrestat uint32) api.Errno

	// FdPrestatDirName is the WASI function to return the path of the pre-opened directory of a file descriptor.
	//
	// * fd - the file descriptor to get the path of the pre-opened directory
	// * path - the offset in `m.Memory` to write the result path
	// * pathLen - the count of bytes to write to `path`
	//   * This should match the uint32le FdPrestatGet writes to offset `resultPrestat`+4
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `path` is an invalid offset due to the memory constraint
	// * wasi.ErrnoNametoolong - if `pathLen` is longer than the actual length of the result path
	//
	// For example, the directory name corresponding with `fd` was "/tmp" and
	//    parameters path=1 pathLen=4 (correct), this function will write the below to `m.Memory`:
	//
	//                  pathLen
	//              +--------------+
	//              |              |
	//   []byte{?, '/', 't', 'm', 'p', ?}
	//       path --^
	//
	// Note: ImportFdPrestatDirName shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See FdPrestatGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_dir_name
	FdPrestatDirName(m api.Module, fd, path, pathLen uint32) api.Errno
	// TODO: FdPrestatDirName may have to return ErrnoNotdir if the type of the prestat data of `fd` is not a PrestatDir.

	// FdPwrite is the WASI function named FunctionFdPwrite
	FdPwrite(m api.Module, fd, iovs uint32, offset uint64, resultNwritten uint32) api.Errno

	// FdRead is the WASI function to read from a file descriptor.
	//
	// * fd - an opened file descriptor to read data from
	// * iovs - the offset in `m.Memory` to read offset, size pairs representing where to write file data.
	//   * Both offset and length are encoded as uint32le.
	// * iovsCount - the count of memory offset, size pairs to read sequentially starting at iovs.
	// * resultSize - the offset in `m.Memory` to write the number of bytes read
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `iovs` or `resultSize` contain an invalid offset due to the memory constraint
	// * wasi.ErrnoIo - if an IO related error happens during the operation
	//
	// For example, this function needs to first read `iovs` to determine where to write contents. If
	//    parameters iovs=1 iovsCount=2, this function reads two offset/length pairs from `m.Memory`:
	//
	//                      iovs[0]                  iovs[1]
	//              +---------------------+   +--------------------+
	//              | uint32le    uint32le|   |uint32le    uint32le|
	//              +---------+  +--------+   +--------+  +--------+
	//              |         |  |        |   |        |  |        |
	//    []byte{?, 18, 0, 0, 0, 4, 0, 0, 0, 23, 0, 0, 0, 2, 0, 0, 0, ?... }
	//       iovs --^            ^            ^           ^
	//              |            |            |           |
	//     offset --+   length --+   offset --+  length --+
	//
	// If the contents of the `fd` parameter was "wazero" (6 bytes) and
	//    parameter resultSize=26, this function writes the below to `m.Memory`:
	//
	//                       iovs[0].length        iovs[1].length
	//                      +--------------+       +----+       uint32le
	//                      |              |       |    |      +--------+
	//   []byte{ 0..16, ?, 'w', 'a', 'z', 'e', ?, 'r', 'o', ?, 6, 0, 0, 0 }
	//     iovs[0].offset --^                      ^           ^
	//                            iovs[1].offset --+           |
	//                                            resultSize --+
	//
	// Note: ImportFdRead shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `readv` in POSIX.
	// See FdWrite
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_read
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#iovec
	// See https://linux.die.net/man/3/readv
	FdRead(m api.Module, fd, iovs, iovsCount, resultSize uint32) api.Errno

	// FdReaddir is the WASI function named FunctionFdReaddir
	FdReaddir(m api.Module, fd, buf, bufLen uint32, cookie uint64, resultBufused uint32) api.Errno

	// FdRenumber is the WASI function named FunctionFdRenumber
	FdRenumber(m api.Module, fd, to uint32) api.Errno

	// FdSeek is the WASI function to move the offset of a file descriptor.
	//
	// * fd: the file descriptor to move the offset of
	// * offset: the signed int64, which is encoded as uint64, input argument to `whence`, which results in a new offset
	// * whence: the operator that creates the new offset, given `offset` bytes
	//   * If io.SeekStart, new offset == `offset`.
	//   * If io.SeekCurrent, new offset == existing offset + `offset`.
	//   * If io.SeekEnd, new offset == file size of `fd` + `offset`.
	// * resultNewoffset: the offset in `m.Memory` to write the new offset to, relative to start of the file
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `resultNewoffset` is an invalid offset in `m.Memory` due to the memory constraint
	// * wasi.ErrnoInval - if `whence` is an invalid value
	// * wasi.ErrnoIo - if other error happens during the operation of the underying file system
	//
	// For example, if fd 3 is a file with offset 0, and
	//   parameters fd=3, offset=4, whence=0 (=io.SeekStart), resultNewOffset=1,
	//   this function writes the below to `m.Memory`:
	//
	//                           uint64le
	//                    +--------------------+
	//                    |                    |
	//          []byte{?, 4, 0, 0, 0, 0, 0, 0, 0, ? }
	//  resultNewoffset --^
	//
	// See io.Seeker
	// Note: ImportFdSeek shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `lseek` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_seek
	// See https://linux.die.net/man/3/lseek
	FdSeek(m api.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) api.Errno

	// FdSync is the WASI function named FunctionFdSync
	FdSync(m api.Module, fd uint32) api.Errno

	// FdTell is the WASI function named FunctionFdTell
	FdTell(m api.Module, fd, resultOffset uint32) api.Errno

	// FdWrite is the WASI function to write to a file descriptor.
	//
	// * fd - an opened file descriptor to write data to
	// * iovs - the offset in `m.Memory` to read offset, size pairs representing the data to write to `fd`
	//   * Both offset and length are encoded as uint32le.
	// * iovsCount - the count of memory offset, size pairs to read sequentially starting at iovs.
	// * resultSize - the offset in `m.Memory` to write the number of bytes written
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `iovs` or `resultSize` contain an invalid offset due to the memory constraint
	// * wasi.ErrnoIo - if an IO related error happens during the operation
	//
	// For example, this function needs to first read `iovs` to determine what to write to `fd`. If
	//    parameters iovs=1 iovsCount=2, this function reads two offset/length pairs from `m.Memory`:
	//
	//                      iovs[0]                  iovs[1]
	//              +---------------------+   +--------------------+
	//              | uint32le    uint32le|   |uint32le    uint32le|
	//              +---------+  +--------+   +--------+  +--------+
	//              |         |  |        |   |        |  |        |
	//    []byte{?, 18, 0, 0, 0, 4, 0, 0, 0, 23, 0, 0, 0, 2, 0, 0, 0, ?... }
	//       iovs --^            ^            ^           ^
	//              |            |            |           |
	//     offset --+   length --+   offset --+  length --+
	//
	// This function reads those chunks `m.Memory` into the `fd` sequentially.
	//
	//                       iovs[0].length        iovs[1].length
	//                      +--------------+       +----+
	//                      |              |       |    |
	//   []byte{ 0..16, ?, 'w', 'a', 'z', 'e', ?, 'r', 'o', ? }
	//     iovs[0].offset --^                      ^
	//                            iovs[1].offset --+
	//
	// Since "wazero" was written, if parameter resultSize=26, this function writes the below to `m.Memory`:
	//
	//                      uint32le
	//                     +--------+
	//                     |        |
	//   []byte{ 0..24, ?, 6, 0, 0, 0', ? }
	//        resultSize --^
	//
	// Note: ImportFdWrite shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `writev` in POSIX.
	// See FdRead
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#ciovec
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_write
	// See https://linux.die.net/man/3/writev
	FdWrite(m api.Module, fd, iovs, iovsCount, resultSize uint32) api.Errno

	// PathCreateDirectory is the WASI function named FunctionPathCreateDirectory
	PathCreateDirectory(m api.Module, fd, path, pathLen uint32) api.Errno

	// PathFilestatGet is the WASI function named FunctionPathFilestatGet
	PathFilestatGet(m api.Module, fd, flags, path, pathLen, resultBuf uint32) api.Errno

	// PathFilestatSetTimes is the WASI function named FunctionPathFilestatSetTimes
	PathFilestatSetTimes(m api.Module, fd, flags, path, pathLen uint32, atim, mtime uint64, fstFlags uint32) api.Errno

	// PathLink is the WASI function named FunctionPathLink
	PathLink(m api.Module, oldFd, oldFlags, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) api.Errno

	// PathOpen is the WASI function to open a file or directory. This returns ErrnoBadf if the fd is invalid.
	//
	// * fd - the file descriptor of a directory that `path` is relative to
	// * dirflags - flags to indicate how to resolve `path`
	// * path - the offset in `m.Memory` to read the path string from
	// * pathLen - the length of `path`
	// * oFlags - the open flags to indicate the method by which to open the file
	// * fsRightsBase - the rights of the newly created file descriptor for `path`
	// * fsRightsInheriting - the rights of the file descriptors derived from the newly created file descriptor for `path`
	// * fdFlags - the file descriptor flags
	// * resultOpenedFd - the offset in `m.Memory` to write the newly created file descriptor to.
	//     * The result FD value is guaranteed to be less than 2**31
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `resultOpenedFd` contains an invalid offset due to the memory constraint
	// * wasi.ErrnoNoent - if `path` does not exist.
	// * wasi.ErrnoExist - if `path` exists, while `oFlags` requires that it must not.
	// * wasi.ErrnoNotdir - if `path` is not a directory, while `oFlags` requires that it must be.
	// * wasi.ErrnoIo - if other error happens during the operation of the underying file system.
	//
	// For example, this function needs to first read `path` to determine the file to open.
	//    If parameters `path` = 1, `pathLen` = 6, and the path is "wazero", PathOpen reads the path from `m.Memory`:
	//
	//                   pathLen
	//               +------------------------+
	//               |                        |
	//   []byte{ ?, 'w', 'a', 'z', 'e', 'r', 'o', ?... }
	//        path --^
	//
	// Then, if parameters resultOpenedFd = 8, and this function opened a new file descriptor 5 with the given flags,
	// this function writes the blow to `m.Memory`:
	//
	//                          uint32le
	//                         +--------+
	//                         |        |
	//        []byte{ 0..6, ?, 5, 0, 0, 0, ?}
	//        resultOpenedFd --^
	//
	// Note: ImportPathOpen shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `openat` in POSIX.
	// Note: The returned file descriptor is not guaranteed to be the lowest-numbered file
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_open
	// See https://linux.die.net/man/3/openat
	PathOpen(m api.Module, fd, dirflags, path, pathLen, oflags uint32, fsRightsBase, fsRightsInheriting uint32, fdflags, resultOpenedFd uint32) api.Errno

	// PathReadlink is the WASI function named FunctionPathReadlink
	PathReadlink(m api.Module, fd, path, pathLen, buf, bufLen, resultBufused uint32) api.Errno

	// PathRemoveDirectory is the WASI function named FunctionPathRemoveDirectory
	PathRemoveDirectory(m api.Module, fd, path, pathLen uint32) api.Errno

	// PathRename is the WASI function named FunctionPathRename
	PathRename(m api.Module, fd, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) api.Errno

	// PathSymlink is the WASI function named FunctionPathSymlink
	PathSymlink(m api.Module, oldPath, oldPathLen, fd, newPath, newPathLen uint32) api.Errno

	// PathUnlinkFile is the WASI function named FunctionPathUnlinkFile
	PathUnlinkFile(m api.Module, fd, path, pathLen uint32) api.Errno

	// PollOneoff is the WASI function named FunctionPollOneoff
	PollOneoff(m api.Module, in, out, nsubscriptions, resultNevents uint32) api.Errno

	// ProcExit is the WASI function that terminates the execution of the module with an exit code.
	// An exit code of 0 indicates successful termination. The meanings of other values are not defined by WASI.
	//
	// * rval - The exit code.
	//
	// In wazero, this calls api.Module CloseWithExitCode.
	//
	// Note: ImportProcExit shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
	ProcExit(m api.Module, rval uint32)

	// ProcRaise is the WASI function named FunctionProcRaise
	ProcRaise(m api.Module, sig uint32) api.Errno

	// SchedYield is the WASI function named FunctionSchedYield
	SchedYield(m api.Module) api.Errno

	// RandomGet is the WASI function named FunctionRandomGet that write random data in buffer (rand.Read()).
	//
	// * buf - is the m.Memory offset to write random values
	// * bufLen - size of random data in bytes
	//
	// For example, if underlying random source was seeded like `rand.NewSource(42)`, we expect `m.Memory` to contain:
	//
	//                             bufLen (5)
	//                    +--------------------------+
	//                    |                        	 |
	//          []byte{?, 0x53, 0x8c, 0x7f, 0x96, 0xb1, ?}
	//              buf --^
	//
	// Note: ImportRandomGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-bufLen-size---errno
	RandomGet(m api.Module, buf, bufLen uint32) api.Errno

	// SockRecv is the WASI function named FunctionSockRecv
	SockRecv(m api.Module, fd, riData, riDataCount, riFlags, resultRoDataLen, resultRoFlags uint32) api.Errno

	// SockSend is the WASI function named FunctionSockSend
	SockSend(m api.Module, fd, siData, siDataCount, siFlags, resultSoDataLen uint32) api.Errno

	// SockShutdown is the WASI function named FunctionSockShutdown
	SockShutdown(m api.Module, fd, how uint32) api.Errno
}

type wasiAPI struct {
	// timeNowUnixNano is mutable for testing
	timeNowUnixNano func() uint64
	randSource      func([]byte) error
}

// SnapshotPreview1Functions returns all go functions that implement SnapshotPreview1.
// These should be exported in the module named "wasi_snapshot_preview1".
// See internalwasm.NewHostModule
func SnapshotPreview1Functions() (a *wasiAPI, nameToGoFunc map[string]interface{}) {
	a = NewAPI()
	// Note: these are ordered per spec for consistency even if the resulting map can't guarantee that.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#functions
	nameToGoFunc = map[string]interface{}{
		FunctionArgsGet:              a.ArgsGet,
		FunctionArgsSizesGet:         a.ArgsSizesGet,
		FunctionEnvironGet:           a.EnvironGet,
		FunctionEnvironSizesGet:      a.EnvironSizesGet,
		FunctionClockResGet:          a.ClockResGet,
		FunctionClockTimeGet:         a.ClockTimeGet,
		FunctionFdAdvise:             a.FdAdvise,
		FunctionFdAllocate:           a.FdAllocate,
		FunctionFdClose:              a.FdClose,
		FunctionFdDatasync:           a.FdDatasync,
		FunctionFdFdstatGet:          a.FdFdstatGet,
		FunctionFdFdstatSetFlags:     a.FdFdstatSetFlags,
		FunctionFdFdstatSetRights:    a.FdFdstatSetRights,
		FunctionFdFilestatGet:        a.FdFilestatGet,
		FunctionFdFilestatSetSize:    a.FdFilestatSetSize,
		FunctionFdFilestatSetTimes:   a.FdFilestatSetTimes,
		FunctionFdPread:              a.FdPread,
		FunctionFdPrestatGet:         a.FdPrestatGet,
		FunctionFdPrestatDirName:     a.FdPrestatDirName,
		FunctionFdPwrite:             a.FdPwrite,
		FunctionFdRead:               a.FdRead,
		FunctionFdReaddir:            a.FdReaddir,
		FunctionFdRenumber:           a.FdRenumber,
		FunctionFdSeek:               a.FdSeek,
		FunctionFdSync:               a.FdSync,
		FunctionFdTell:               a.FdTell,
		FunctionFdWrite:              a.FdWrite,
		FunctionPathCreateDirectory:  a.PathCreateDirectory,
		FunctionPathFilestatGet:      a.PathFilestatGet,
		FunctionPathFilestatSetTimes: a.PathFilestatSetTimes,
		FunctionPathLink:             a.PathLink,
		FunctionPathOpen:             a.PathOpen,
		FunctionPathReadlink:         a.PathReadlink,
		FunctionPathRemoveDirectory:  a.PathRemoveDirectory,
		FunctionPathRename:           a.PathRename,
		FunctionPathSymlink:          a.PathSymlink,
		FunctionPathUnlinkFile:       a.PathUnlinkFile,
		FunctionPollOneoff:           a.PollOneoff,
		FunctionProcExit:             a.ProcExit,
		FunctionProcRaise:            a.ProcRaise,
		FunctionSchedYield:           a.SchedYield,
		FunctionRandomGet:            a.RandomGet,
		FunctionSockRecv:             a.SockRecv,
		FunctionSockSend:             a.SockSend,
		FunctionSockShutdown:         a.SockShutdown,
	}
	return
}

// ArgsGet implements SnapshotPreview1.ArgsGet
func (a *wasiAPI) ArgsGet(m api.Module, argv, argvBuf uint32) api.Errno {
	sys := sysCtx(m)
	return writeOffsetsAndNullTerminatedValues(m.Memory(), sys.Args(), argv, argvBuf)
}

// ArgsSizesGet implements SnapshotPreview1.ArgsSizesGet
func (a *wasiAPI) ArgsSizesGet(m api.Module, resultArgc, resultArgvBufSize uint32) api.Errno {
	sys := sysCtx(m)
	mem := m.Memory()

	if !mem.WriteUint32Le(resultArgc, uint32(len(sys.Args()))) {
		return api.ErrnoFault
	}
	if !mem.WriteUint32Le(resultArgvBufSize, sys.ArgsSize()) {
		return api.ErrnoFault
	}
	return api.ErrnoSuccess
}

// EnvironGet implements SnapshotPreview1.EnvironGet
func (a *wasiAPI) EnvironGet(m api.Module, environ uint32, environBuf uint32) api.Errno {
	sys := sysCtx(m)
	return writeOffsetsAndNullTerminatedValues(m.Memory(), sys.Environ(), environ, environBuf)
}

// EnvironSizesGet implements SnapshotPreview1.EnvironSizesGet
func (a *wasiAPI) EnvironSizesGet(m api.Module, resultEnvironc uint32, resultEnvironBufSize uint32) api.Errno {
	sys := sysCtx(m)
	mem := m.Memory()

	if !mem.WriteUint32Le(resultEnvironc, uint32(len(sys.Environ()))) {
		return api.ErrnoFault
	}
	if !mem.WriteUint32Le(resultEnvironBufSize, sys.EnvironSize()) {
		return api.ErrnoFault
	}

	return api.ErrnoSuccess
}

// ClockResGet implements SnapshotPreview1.ClockResGet
func (a *wasiAPI) ClockResGet(m api.Module, id uint32, resultResolution uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// ClockTimeGet implements SnapshotPreview1.ClockTimeGet
func (a *wasiAPI) ClockTimeGet(m api.Module, id uint32, precision uint64, resultTimestamp uint32) api.Errno {
	// TODO: id and precision are currently ignored.
	if !m.Memory().WriteUint64Le(resultTimestamp, a.timeNowUnixNano()) {
		return api.ErrnoFault
	}
	return api.ErrnoSuccess
}

// FdAdvise implements SnapshotPreview1.FdAdvise
func (a *wasiAPI) FdAdvise(m api.Module, fd uint32, offset, len uint64, resultAdvice uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdAllocate implements SnapshotPreview1.FdAllocate
func (a *wasiAPI) FdAllocate(m api.Module, fd uint32, offset, len uint64) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdClose implements SnapshotPreview1.FdClose
func (a *wasiAPI) FdClose(m api.Module, fd uint32) api.Errno {
	sys := sysCtx(m)

	if ok, err := sys.CloseFile(fd); err != nil {
		return api.ErrnoIo
	} else if !ok {
		return api.ErrnoBadf
	}

	return api.ErrnoSuccess
}

// FdDatasync implements SnapshotPreview1.FdDatasync
func (a *wasiAPI) FdDatasync(m api.Module, fd uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFdstatGet implements SnapshotPreview1.FdFdstatGet
func (a *wasiAPI) FdFdstatGet(m api.Module, fd uint32, resultStat uint32) api.Errno {
	sys := sysCtx(m)

	if _, ok := sys.OpenedFile(fd); !ok {
		return api.ErrnoBadf
	}
	return api.ErrnoSuccess
}

// FdPrestatGet implements SnapshotPreview1.FdPrestatGet
func (a *wasiAPI) FdPrestatGet(m api.Module, fd uint32, resultPrestat uint32) api.Errno {
	sys := sysCtx(m)

	entry, ok := sys.OpenedFile(fd)
	if !ok {
		return api.ErrnoBadf
	}

	// Zero-value 8-bit tag, and 3-byte zero-value paddings, which is uint32le(0) in short.
	if !m.Memory().WriteUint32Le(resultPrestat, uint32(0)) {
		return api.ErrnoFault
	}
	// Write the length of the directory name at offset 4.
	if !m.Memory().WriteUint32Le(resultPrestat+4, uint32(len(entry.Path))) {
		return api.ErrnoFault
	}

	return api.ErrnoSuccess
}

// FdFdstatSetFlags implements SnapshotPreview1.FdFdstatSetFlags
func (a *wasiAPI) FdFdstatSetFlags(m api.Module, fd uint32, flags uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFdstatSetRights implements SnapshotPreview1.FdFdstatSetRights
// Note: This will never be implemented per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
func (a *wasiAPI) FdFdstatSetRights(m api.Module, fd uint32, fsRightsBase, fsRightsInheriting uint64) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatGet implements SnapshotPreview1.FdFilestatGet
func (a *wasiAPI) FdFilestatGet(m api.Module, fd uint32, resultBuf uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatSetSize implements SnapshotPreview1.FdFilestatSetSize
func (a *wasiAPI) FdFilestatSetSize(m api.Module, fd uint32, size uint64) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatSetTimes implements SnapshotPreview1.FdFilestatSetTimes
func (a *wasiAPI) FdFilestatSetTimes(m api.Module, fd uint32, atim, mtim uint64, fstFlags uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdPread implements SnapshotPreview1.FdPread
func (a *wasiAPI) FdPread(m api.Module, fd, iovs, iovsCount uint32, offset uint64, resultNread uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdPrestatDirName implements SnapshotPreview1.FdPrestatDirName
func (a *wasiAPI) FdPrestatDirName(m api.Module, fd uint32, pathPtr uint32, pathLen uint32) api.Errno {
	sys := sysCtx(m)

	f, ok := sys.OpenedFile(fd)
	if !ok {
		return api.ErrnoBadf
	}

	// Some runtimes may have another semantics. See internal/wasi/RATIONALE.md
	if uint32(len(f.Path)) < pathLen {
		return api.ErrnoNametoolong
	}

	// TODO: FdPrestatDirName may have to return ErrnoNotdir if the type of the prestat data of `fd` is not a PrestatDir.
	if !m.Memory().Write(pathPtr, []byte(f.Path)[:pathLen]) {
		return api.ErrnoFault
	}
	return api.ErrnoSuccess
}

// FdPwrite implements SnapshotPreview1.FdPwrite
func (a *wasiAPI) FdPwrite(m api.Module, fd, iovs, iovsCount uint32, offset uint64, resultNwritten uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdRead implements SnapshotPreview1.FdRead
func (a *wasiAPI) FdRead(m api.Module, fd, iovs, iovsCount, resultSize uint32) api.Errno {
	sys := sysCtx(m)

	var reader io.Reader

	if fd == fdStdin {
		reader = sys.Stdin()
	} else if f, ok := sys.OpenedFile(fd); !ok || f.File == nil {
		return api.ErrnoBadf
	} else {
		reader = f.File
	}

	var nread uint32
	for i := uint32(0); i < iovsCount; i++ {
		iovPtr := iovs + i*8
		offset, ok := m.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return api.ErrnoFault
		}
		l, ok := m.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return api.ErrnoFault
		}
		b, ok := m.Memory().Read(offset, l)
		if !ok {
			return api.ErrnoFault
		}
		n, err := reader.Read(b)
		nread += uint32(n)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return api.ErrnoIo
		}
	}
	if !m.Memory().WriteUint32Le(resultSize, nread) {
		return api.ErrnoFault
	}
	return api.ErrnoSuccess
}

// FdReaddir implements SnapshotPreview1.FdReaddir
func (a *wasiAPI) FdReaddir(m api.Module, fd, buf, bufLen uint32, cookie uint64, resultBufused uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdRenumber implements SnapshotPreview1.FdRenumber
func (a *wasiAPI) FdRenumber(m api.Module, fd, to uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdSeek implements SnapshotPreview1.FdSeek
func (a *wasiAPI) FdSeek(m api.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) api.Errno {
	sys := sysCtx(m)

	var seeker io.Seeker
	// Check to see if the file descriptor is available
	if f, ok := sys.OpenedFile(fd); !ok || f.File == nil {
		return api.ErrnoBadf
		// fs.FS doesn't declare io.Seeker, but implementations such as os.File implement it.
	} else if seeker, ok = f.File.(io.Seeker); !ok {
		return api.ErrnoBadf
	}

	if whence > io.SeekEnd /* exceeds the largest valid whence */ {
		return api.ErrnoInval
	}
	newOffset, err := seeker.Seek(int64(offset), int(whence))
	if err != nil {
		return api.ErrnoIo
	}

	if !m.Memory().WriteUint32Le(resultNewoffset, uint32(newOffset)) {
		return api.ErrnoFault
	}

	return api.ErrnoSuccess
}

// FdSync implements SnapshotPreview1.FdSync
func (a *wasiAPI) FdSync(m api.Module, fd uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdTell implements SnapshotPreview1.FdTell
func (a *wasiAPI) FdTell(m api.Module, fd, resultOffset uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// FdWrite implements SnapshotPreview1.FdWrite
func (a *wasiAPI) FdWrite(m api.Module, fd, iovs, iovsCount, resultSize uint32) api.Errno {
	sys := sysCtx(m)

	var writer io.Writer

	switch fd {
	case fdStdout:
		writer = sys.Stdout()
	case fdStderr:
		writer = sys.Stderr()
	default:
		// Check to see if the file descriptor is available
		if f, ok := sys.OpenedFile(fd); !ok || f.File == nil {
			return api.ErrnoBadf
			// fs.FS doesn't declare io.Writer, but implementations such as os.File implement it.
		} else if writer, ok = f.File.(io.Writer); !ok {
			return api.ErrnoBadf
		}
	}

	var nwritten uint32
	for i := uint32(0); i < iovsCount; i++ {
		iovPtr := iovs + i*8
		offset, ok := m.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return api.ErrnoFault
		}
		l, ok := m.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return api.ErrnoFault
		}
		b, ok := m.Memory().Read(offset, l)
		if !ok {
			return api.ErrnoFault
		}
		n, err := writer.Write(b)
		if err != nil {
			return api.ErrnoIo
		}
		nwritten += uint32(n)
	}
	if !m.Memory().WriteUint32Le(resultSize, nwritten) {
		return api.ErrnoFault
	}
	return api.ErrnoSuccess
}

// PathCreateDirectory implements SnapshotPreview1.PathCreateDirectory
func (a *wasiAPI) PathCreateDirectory(m api.Module, fd, path, pathLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathFilestatGet implements SnapshotPreview1.PathFilestatGet
func (a *wasiAPI) PathFilestatGet(m api.Module, fd, flags, path, pathLen, resultBuf uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathFilestatSetTimes implements SnapshotPreview1.PathFilestatSetTimes
func (a *wasiAPI) PathFilestatSetTimes(m api.Module, fd, flags, path, pathLen uint32, atim, mtime uint64, fstFlags uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathLink implements SnapshotPreview1.PathLink
func (a *wasiAPI) PathLink(m api.Module, oldFd, oldFlags, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathOpen implements SnapshotPreview1.PathOpen
// Note: Rights will never be implemented per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
func (a *wasiAPI) PathOpen(m api.Module, fd, dirflags, pathPtr, pathLen, oflags uint32, fsRightsBase,
	fsRightsInheriting uint64, fdflags, resultOpenedFd uint32) (errno api.Errno) {
	sys := sysCtx(m)

	dir, ok := sys.OpenedFile(fd)
	if !ok || dir.FS == nil {
		return api.ErrnoBadf
	}

	b, ok := m.Memory().Read(pathPtr, pathLen)
	if !ok {
		return api.ErrnoFault
	}

	// TODO: Consider dirflags and oflags. Also, allow non-read-only open based on config about the mount.
	// Ex. allow os.O_RDONLY, os.O_WRONLY, or os.O_RDWR either by config flag or pattern on filename
	// See #390
	entry, errno := openFileEntry(dir.FS, path.Join(dir.Path, string(b)))
	if errno != api.ErrnoSuccess {
		return errno
	}

	if newFD, ok := sys.OpenFile(entry); !ok {
		_ = entry.File.Close()
		return api.ErrnoIo
	} else if !m.Memory().WriteUint32Le(resultOpenedFd, newFD) {
		_ = entry.File.Close()
		return api.ErrnoFault
	}
	return api.ErrnoSuccess
}

// PathReadlink implements SnapshotPreview1.PathReadlink
func (a *wasiAPI) PathReadlink(m api.Module, fd, path, pathLen, buf, bufLen, resultBufused uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathRemoveDirectory implements SnapshotPreview1.PathRemoveDirectory
func (a *wasiAPI) PathRemoveDirectory(m api.Module, fd, path, pathLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathRename implements SnapshotPreview1.PathRename
func (a *wasiAPI) PathRename(m api.Module, fd, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathSymlink implements SnapshotPreview1.PathSymlink
func (a *wasiAPI) PathSymlink(m api.Module, oldPath, oldPathLen, fd, newPath, newPathLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PathUnlinkFile implements SnapshotPreview1.PathUnlinkFile
func (a *wasiAPI) PathUnlinkFile(m api.Module, fd, path, pathLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// PollOneoff implements SnapshotPreview1.PollOneoff
func (a *wasiAPI) PollOneoff(m api.Module, in, out, nsubscriptions, resultNevents uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// ProcExit implements SnapshotPreview1.ProcExit
func (a *wasiAPI) ProcExit(m api.Module, exitCode uint32) {
	_ = m.CloseWithExitCode(exitCode)
}

// ProcRaise implements SnapshotPreview1.ProcRaise
func (a *wasiAPI) ProcRaise(m api.Module, sig uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// SchedYield implements SnapshotPreview1.SchedYield
func (a *wasiAPI) SchedYield(m api.Module) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// RandomGet implements SnapshotPreview1.RandomGet
func (a *wasiAPI) RandomGet(m api.Module, buf uint32, bufLen uint32) (errno api.Errno) {
	randomBytes := make([]byte, bufLen)
	err := a.randSource(randomBytes)
	if err != nil {
		// TODO: handle different errors that syscal to entropy source can return
		return api.ErrnoIo
	}

	if !m.Memory().Write(buf, randomBytes) {
		return api.ErrnoFault
	}

	return api.ErrnoSuccess
}

// SockRecv implements SnapshotPreview1.SockRecv
func (a *wasiAPI) SockRecv(m api.Module, fd, riData, riDataCount, riFlags, resultRoDataLen, resultRoFlags uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// SockSend implements SnapshotPreview1.SockSend
func (a *wasiAPI) SockSend(m api.Module, fd, siData, siDataCount, siFlags, resultSoDataLen uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

// SockShutdown implements SnapshotPreview1.SockShutdown
func (a *wasiAPI) SockShutdown(m api.Module, fd, how uint32) api.Errno {
	return api.ErrnoNosys // stubbed for GrainLang per #271
}

const (
	fdStdin  = 0
	fdStdout = 1
	fdStderr = 2
)

// NewAPI is exported for benchmarks
func NewAPI() *wasiAPI {
	return &wasiAPI{
		timeNowUnixNano: func() uint64 {
			return uint64(time.Now().UnixNano())
		},
		randSource: func(p []byte) error {
			_, err := crand.Read(p)
			return err
		},
	}
}

func sysCtx(m api.Module) *internalwasm.SysContext {
	if internal, ok := m.(*internalwasm.ModuleContext); !ok {
		panic(fmt.Errorf("unsupported wasm.Module implementation: %v", m))
	} else {
		return internal.Sys()
	}
}

func openFileEntry(rootFS fs.FS, pathName string) (*internalwasm.FileEntry, api.Errno) {
	f, err := rootFS.Open(pathName)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, api.ErrnoNoent
		case errors.Is(err, fs.ErrExist):
			return nil, api.ErrnoExist
		default:
			return nil, api.ErrnoIo
		}
	}

	// TODO: verify if oflags is a directory and fail with wasi.ErrnoNotdir if not
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-oflags-flagsu16

	return &internalwasm.FileEntry{Path: pathName, FS: rootFS, File: f}, api.ErrnoSuccess
}

func writeOffsetsAndNullTerminatedValues(mem api.Memory, values []string, offsets, bytes uint32) api.Errno {
	for _, value := range values {
		// Write current offset and advance it.
		if !mem.WriteUint32Le(offsets, bytes) {
			return api.ErrnoFault
		}
		offsets += 4 // size of uint32

		// Write the next value to memory with a NUL terminator
		if !mem.Write(bytes, []byte(value)) {
			return api.ErrnoFault
		}
		bytes += uint32(len(value))
		if !mem.WriteByte(bytes, 0) {
			return api.ErrnoFault
		}
		bytes++
	}

	return api.ErrnoSuccess
}
