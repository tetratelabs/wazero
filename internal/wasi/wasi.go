package internalwasi

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"strings"
	"time"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
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

// SnapshotPreview1 includes all host functions to export for WASI version wasi.ModuleSnapshotPreview1.
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
// In WebAssembly 1.0 (20191205), there may be up to one Memory per store, which means wasm.Memory is always the
// wasm.Store Memories index zero: `store.Memories[0].Buffer`
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
// See https://github.com/WebAssembly/WASI/issues/215
// See https://wwa.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-instances%E2%91%A0.
type SnapshotPreview1 interface {
	// ArgsGet is the WASI function that reads command-line argument data (Args).
	//
	// There are two parameters. Both are offsets in wasm.Module Memory. If either are invalid due to
	// memory constraints, this returns ErrnoFault.
	//
	// * argv - is the offset to begin writing argument offsets in uint32 little-endian encoding.
	//   * ArgsSizesGet result argc * 4 bytes are written to this offset
	// * argvBuf - is the offset to write the null terminated arguments to ctx.Memory
	//   * ArgsSizesGet result argv_buf_size bytes are written to this offset
	//
	// For example, if ArgsSizesGet wrote argc=2 and argvBufSize=5 for arguments: "a" and "bc"
	//    parameters argv=7 and argvBuf=1, this function writes the below to `ctx.Memory`:
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
	ArgsGet(ctx wasm.Module, argv, argvBuf uint32) wasi.Errno

	// ArgsSizesGet is the WASI function named FunctionArgsSizesGet that reads command-line argument data (Args)
	// sizes.
	//
	// There are two result parameters: these are offsets in the wasm.Module Memory to write
	// corresponding sizes in uint32 little-endian encoding. If either are invalid due to memory constraints, this
	// returns ErrnoFault.
	//
	// * resultArgc - is the offset to write the argument count to ctx.Memory
	// * resultArgvBufSize - is the offset to write the null-terminated argument length to ctx.Memory
	//
	// For example, if Args are []string{"a","bc"} and
	//    parameters resultArgc=1 and resultArgvBufSize=6, this function writes the below to `ctx.Memory`:
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
	ArgsSizesGet(ctx wasm.Module, resultArgc, resultArgvBufSize uint32) wasi.Errno

	// EnvironGet is the WASI function named FunctionEnvironGet that reads environment variables. (Environ)
	//
	// There are two parameters. Both are offsets in wasm.Module Memory. If either are invalid due to
	// memory constraints, this returns ErrnoFault.
	//
	// * environ - is the offset to begin writing environment variables offsets in uint32 little-endian encoding.
	//   * EnvironSizesGet result environc * 4 bytes are written to this offset
	// * environBuf - is the offset to write the environment variables to ctx.Memory
	//   * the format is the same as os.Environ, null terminated "key=val" entries
	//   * EnvironSizesGet result environBufSize bytes are written to this offset
	//
	// For example, if EnvironSizesGet wrote environc=2 and environBufSize=9 for environment variables: "a=b", "b=cd"
	//   and parameters environ=11 and environBuf=1, this function writes the below to `ctx.Memory`:
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
	EnvironGet(ctx wasm.Module, environ, environBuf uint32) wasi.Errno

	// EnvironSizesGet is the WASI function named FunctionEnvironSizesGet that reads environment variable
	// (Environ) sizes.
	//
	// There are two result parameters: these are offsets in the wasi.Module Memory to write
	// corresponding sizes in uint32 little-endian encoding. If either are invalid due to memory constraints, this
	// returns ErrnoFault.
	//
	// * resultEnvironc - is the offset to write the environment variable count to ctx.Memory
	// * resultEnvironBufSize - is the offset to write the null-terminated environment variable length to ctx.Memory
	//
	// For example, if Environ is []string{"a=b","b=cd"} and
	//    parameters resultEnvironc=1 and resultEnvironBufSize=6, this function writes the below to `ctx.Memory`:
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
	EnvironSizesGet(ctx wasm.Module, resultEnvironc, resultEnvironBufSize uint32) wasi.Errno

	// ClockResGet is the WASI function named FunctionClockResGet and is stubbed for GrainLang per #271
	ClockResGet(ctx wasm.Module, id uint32, resultResolution uint32) wasi.Errno

	// ClockTimeGet is the WASI function named FunctionClockTimeGet that returns the time value of a clock (time.Now).
	//
	// * id - The clock id for which to return the time.
	// * precision - The maximum lag (exclusive) that the returned time value may have, compared to its actual value.
	// * resultTimestamp - the offset to write the timestamp to ctx.Memory
	//   * the timestamp is epoch nanoseconds encoded as a uint64 little-endian encoding.
	//
	// For example, if time.Now returned exactly midnight UTC 2022-01-01 (1640995200000000000), and
	//   parameters resultTimestamp=1, this function writes the below to `ctx.Memory`:
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
	ClockTimeGet(ctx wasm.Module, id uint32, precision uint64, resultTimestamp uint32) wasi.Errno

	// FdAdvise is the WASI function named FunctionFdAdvise and is stubbed for GrainLang per #271
	FdAdvise(ctx wasm.Module, fd uint32, offset, len uint64, resultAdvice uint32) wasi.Errno

	// FdAllocate is the WASI function named FunctionFdAllocate and is stubbed for GrainLang per #271
	FdAllocate(ctx wasm.Module, fd uint32, offset, len uint64, resultAdvice uint32) wasi.Errno

	// FdClose is the WASI function to close a file descriptor. This returns ErrnoBadf if the fd is invalid.
	//
	// * fd - the file descriptor to close
	//
	// Note: ImportFdClose shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `close` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_close
	// See https://linux.die.net/man/3/close
	FdClose(ctx wasm.Module, fd uint32) wasi.Errno

	// FdDatasync is the WASI function named FunctionFdDatasync and is stubbed for GrainLang per #271
	FdDatasync(ctx wasm.Module, fd uint32) wasi.Errno

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
	//    parameter resultFdstat=1, this function writes the below to `ctx.Memory`:
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
	FdFdstatGet(ctx wasm.Module, fd, resultFdstat uint32) wasi.Errno

	// FdFdstatSetFlags is the WASI function named FunctionFdFdstatSetFlags and is stubbed for GrainLang per #271
	FdFdstatSetFlags(ctx wasm.Module, fd uint32, flags uint32) wasi.Errno

	// FdFdstatSetRights is the WASI function named FunctionFdFdstatSetRights and is stubbed for GrainLang per #271
	FdFdstatSetRights(ctx wasm.Module, fd uint32, fsRightsBase, fsRightsInheriting uint64) wasi.Errno

	// FdFilestatGet is the WASI function named FunctionFdFilestatGet
	FdFilestatGet(ctx wasm.Module, fd uint32, resultBuf uint32) wasi.Errno

	// FdFilestatSetSize is the WASI function named FunctionFdFilestatSetSize
	FdFilestatSetSize(ctx wasm.Module, fd uint32, size uint64) wasi.Errno

	// FdFilestatSetTimes is the WASI function named FunctionFdFilestatSetTimes
	FdFilestatSetTimes(ctx wasm.Module, fd uint32, atim, mtim uint64, fstFlags uint32) wasi.Errno

	// FdPread is the WASI function named FunctionFdPread
	FdPread(ctx wasm.Module, fd, iovs uint32, offset uint64, resultNread uint32) wasi.Errno

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
	//    parameter resultPrestat=1, this function writes the below to `ctx.Memory`:
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
	FdPrestatGet(ctx wasm.Module, fd uint32, resultPrestat uint32) wasi.Errno

	// FdPrestatDirName is the WASI function to return the path of the pre-opened directory of a file descriptor.
	//
	// * fd - the file descriptor to get the path of the pre-opened directory
	// * path - the offset in `ctx.Memory` to write the result path
	// * pathLen - the count of bytes to write to `path`
	//   * This should match the uint32le FdPrestatGet writes to offset `resultPrestat`+4
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `path` is an invalid offset due to the memory constraint
	// * wasi.ErrnoNametoolong - if `pathLen` is longer than the actual length of the result path
	//
	// For example, the directory name corresponding with `fd` was "/tmp" and
	//    parameters path=1 pathLen=4 (correct), this function will write the below to `ctx.Memory`:
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
	FdPrestatDirName(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno
	// TODO: FdPrestatDirName may have to return ErrnoNotdir if the type of the prestat data of `fd` is not a PrestatDir.

	// FdPwrite is the WASI function named FunctionFdPwrite
	FdPwrite(ctx wasm.Module, fd, iovs uint32, offset uint64, resultNwritten uint32) wasi.Errno

	// FdRead is the WASI function to read from a file descriptor.
	//
	// * fd - an opened file descriptor to read data from
	// * iovs - the offset in `ctx.Memory` to read offset, size pairs representing where to write file data.
	//   * Both offset and length are encoded as uint32le.
	// * iovsCount - the count of memory offset, size pairs to read sequentially starting at iovs.
	// * resultSize - the offset in `ctx.Memory` to write the number of bytes read
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `iovs` or `resultSize` contain an invalid offset due to the memory constraint
	// * wasi.ErrnoIo - if an IO related error happens during the operation
	//
	// For example, this function needs to first read `iovs` to determine where to write contents. If
	//    parameters iovs=1 iovsCount=2, this function reads two offset/length pairs from `ctx.Memory`:
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
	//    parameter resultSize=26, this function writes the below to `ctx.Memory`:
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
	FdRead(ctx wasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno

	// FdReaddir is the WASI function named FunctionFdReaddir
	FdReaddir(ctx wasm.Module, fd, buf, bufLen uint32, cookie uint64, resultBufused uint32) wasi.Errno

	// FdRenumber is the WASI function named FunctionFdRenumber
	FdRenumber(ctx wasm.Module, fd, to uint32) wasi.Errno

	// FdSeek is the WASI function to move the offset of a file descriptor.
	//
	// * fd: the file descriptor to move the offset of
	// * offset: the signed int64, which is encoded as uint64, input argument to `whence`, which results in a new offset
	// * whence: the operator that creates the new offset, given `offset` bytes
	//   * If io.SeekStart, new offset == `offset`.
	//   * If io.SeekCurrent, new offset == existing offset + `offset`.
	//   * If io.SeekEnd, new offset == file size of `fd` + `offset`.
	// * resultNewoffset: the offset in `ctx.Memory` to write the new offset to, relative to start of the file
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `resultNewoffset` is an invalid offset in `ctx.Memory` due to the memory constraint
	// * wasi.ErrnoInval - if `whence` is an invalid value
	// * wasi.ErrnoIo - if other error happens during the operation of the underying file system
	//
	// For example, if fd 3 is a file with offset 0, and
	//   parameters fd=3, offset=4, whence=0 (=io.SeekStart), resultNewOffset=1,
	//   this function writes the below to `ctx.Memory`:
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
	FdSeek(ctx wasm.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) wasi.Errno

	// FdSync is the WASI function named FunctionFdSync
	FdSync(ctx wasm.Module, fd uint32) wasi.Errno

	// FdTell is the WASI function named FunctionFdTell
	FdTell(ctx wasm.Module, fd, resultOffset uint32) wasi.Errno

	// FdWrite is the WASI function to write to a file descriptor.
	//
	// * fd - an opened file descriptor to write data to
	// * iovs - the offset in `ctx.Memory` to read offset, size pairs representing the data to write to `fd`
	//   * Both offset and length are encoded as uint32le.
	// * iovsCount - the count of memory offset, size pairs to read sequentially starting at iovs.
	// * resultSize - the offset in `ctx.Memory` to write the number of bytes written
	//
	// The wasi.Errno returned is wasi.ErrnoSuccess except the following error conditions:
	// * wasi.ErrnoBadf - if `fd` is invalid
	// * wasi.ErrnoFault - if `iovs` or `resultSize` contain an invalid offset due to the memory constraint
	// * wasi.ErrnoIo - if an IO related error happens during the operation
	//
	// For example, this function needs to first read `iovs` to determine what to write to `fd`. If
	//    parameters iovs=1 iovsCount=2, this function reads two offset/length pairs from `ctx.Memory`:
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
	// This function reads those chunks `ctx.Memory` into the `fd` sequentially.
	//
	//                       iovs[0].length        iovs[1].length
	//                      +--------------+       +----+
	//                      |              |       |    |
	//   []byte{ 0..16, ?, 'w', 'a', 'z', 'e', ?, 'r', 'o', ? }
	//     iovs[0].offset --^                      ^
	//                            iovs[1].offset --+
	//
	// Since "wazero" was written, if parameter resultSize=26, this function writes the below to `ctx.Memory`:
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
	FdWrite(ctx wasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno

	// PathCreateDirectory is the WASI function named FunctionPathCreateDirectory
	PathCreateDirectory(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno

	// PathFilestatGet is the WASI function named FunctionPathFilestatGet
	PathFilestatGet(ctx wasm.Module, fd, flags, path, pathLen, resultBuf uint32) wasi.Errno

	// PathFilestatSetTimes is the WASI function named FunctionPathFilestatSetTimes
	PathFilestatSetTimes(ctx wasm.Module, fd, flags, path, pathLen uint32, atim, mtime uint64, fstFlags uint32) wasi.Errno

	// PathLink is the WASI function named FunctionPathLink
	PathLink(ctx wasm.Module, oldFd, oldFlags, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) wasi.Errno

	// PathOpen is the WASI function to open a file or directory. This returns ErrnoBadf if the fd is invalid.
	//
	// * fd - the file descriptor of a directory that `path` is relative to
	// * dirflags - flags to indicate how to resolve `path`
	// * path - the offset in `ctx.Memory` to read the path string from
	// * pathLen - the length of `path`
	// * oFlags - the open flags to indicate the method by which to open the file
	// * fsRightsBase - the rights of the newly created file descriptor for `path`
	// * fsRightsInheriting - the rights of the file descriptors derived from the newly created file descriptor for `path`
	// * fdFlags - the file descriptor flags
	// * resultOpenedFd - the offset in `ctx.Memory` to write the newly created file descriptor to.
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
	//    If parameters `path` = 1, `pathLen` = 6, and the path is "wazero", PathOpen reads the path from `ctx.Memory`:
	//
	//                   pathLen
	//               +------------------------+
	//               |                        |
	//   []byte{ ?, 'w', 'a', 'z', 'e', 'r', 'o', ?... }
	//        path --^
	//
	// Then, if parameters resultOpenedFd = 8, and this function opened a new file descriptor 5 with the given flags,
	// this function writes the blow to `ctx.Memory`:
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
	PathOpen(ctx wasm.Module, fd, dirflags, path, pathLen, oflags uint32, fsRightsBase, fsRightsInheriting uint32, fdflags, resultOpenedFd uint32) wasi.Errno

	// PathReadlink is the WASI function named FunctionPathReadlink
	PathReadlink(ctx wasm.Module, fd, path, pathLen, buf, bufLen, resultBufused uint32) wasi.Errno

	// PathRemoveDirectory is the WASI function named FunctionPathRemoveDirectory
	PathRemoveDirectory(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno

	// PathRename is the WASI function named FunctionPathRename
	PathRename(ctx wasm.Module, fd, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) wasi.Errno

	// PathSymlink is the WASI function named FunctionPathSymlink
	PathSymlink(ctx wasm.Module, oldPath, oldPathLen, fd, newPath, newPathLen uint32) wasi.Errno

	// PathUnlinkFile is the WASI function named FunctionPathUnlinkFile
	PathUnlinkFile(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno

	// PollOneoff is the WASI function named FunctionPollOneoff
	PollOneoff(ctx wasm.Module, in, out, nsubscriptions, resultNevents uint32) wasi.Errno

	// ProcExit is the WASI function that terminates the execution of the module with an exit code.
	// An exit code of 0 indicates successful termination. The meanings of other values are not defined by WASI.
	//
	// * rval - The exit code.
	//
	// In wazero, if ProcExit is called, the calling function returns immediately, returning the given exit code as the error.
	// You can get the exit code by casting the error to wasi.ExitCode.
	// See wasi.ExitCode
	//
	// Note: ImportProcExit shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
	ProcExit(rval uint32)

	// ProcRaise is the WASI function named FunctionProcRaise
	ProcRaise(ctx wasm.Module, sig uint32) wasi.Errno

	// SchedYield is the WASI function named FunctionSchedYield
	SchedYield(ctx wasm.Module) wasi.Errno

	// RandomGet is the WASI function named FunctionRandomGet that write random data in buffer (rand.Read()).
	//
	// * buf - is the ctx.Memory offset to write random values
	// * bufLen - size of random data in bytes
	//
	// For example, if underlying random source was seeded like `rand.NewSource(42)`, we expect `ctx.Memory` to contain:
	//
	//                             bufLen (5)
	//                    +--------------------------+
	//                    |                        	 |
	//          []byte{?, 0x53, 0x8c, 0x7f, 0x96, 0xb1, ?}
	//              buf --^
	//
	// Note: ImportRandomGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-bufLen-size---errno
	RandomGet(ctx wasm.Module, buf, bufLen uint32) wasi.Errno

	// SockRecv is the WASI function named FunctionSockRecv
	SockRecv(ctx wasm.Module, fd, riData, riDataCount, riFlags, resultRoDataLen, resultRoFlags uint32) wasi.Errno

	// SockSend is the WASI function named FunctionSockSend
	SockSend(ctx wasm.Module, fd, siData, siDataCount, siFlags, resultSoDataLen uint32) wasi.Errno

	// SockShutdown is the WASI function named FunctionSockShutdown
	SockShutdown(ctx wasm.Module, fd, how uint32) wasi.Errno
}

type wasiAPI struct {
	// cfg is the default configuration to use when there is no context.Context override (ConfigContextKey).
	cfg *Config
}

// SnapshotPreview1Functions returns all go functions that implement SnapshotPreview1.
// These should be exported in the module named wasi.ModuleSnapshotPreview1.
// See internalwasm.NewHostModule
func SnapshotPreview1Functions(config *Config) (nameToGoFunc map[string]interface{}) {
	a := NewAPI(config)
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
func (a *wasiAPI) ArgsGet(ctx wasm.Module, argv, argvBuf uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	for _, arg := range cfg.args.nullTerminatedValues {
		if !ctx.Memory().WriteUint32Le(argv, argvBuf) {
			return wasi.ErrnoFault
		}
		argv += 4 // size of uint32
		if !ctx.Memory().Write(argvBuf, arg) {
			return wasi.ErrnoFault
		}
		argvBuf += uint32(len(arg))
	}

	return wasi.ErrnoSuccess
}

// ArgsSizesGet implements SnapshotPreview1.ArgsSizesGet
func (a *wasiAPI) ArgsSizesGet(ctx wasm.Module, resultArgc, resultArgvBufSize uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	if !ctx.Memory().WriteUint32Le(resultArgc, uint32(len(cfg.args.nullTerminatedValues))) {
		return wasi.ErrnoFault
	}
	if !ctx.Memory().WriteUint32Le(resultArgvBufSize, cfg.args.totalBufSize) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// EnvironGet implements SnapshotPreview1.EnvironGet
func (a *wasiAPI) EnvironGet(ctx wasm.Module, environ uint32, environBuf uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	// w.environ holds the environment variables in the form of "key=val\x00", so just copies it to the linear memory.
	for _, env := range cfg.environ.nullTerminatedValues {
		if !ctx.Memory().WriteUint32Le(environ, environBuf) {
			return wasi.ErrnoFault
		}
		environ += 4 // size of uint32
		if !ctx.Memory().Write(environBuf, env) {
			return wasi.ErrnoFault
		}
		environBuf += uint32(len(env))
	}

	return wasi.ErrnoSuccess
}

// EnvironSizesGet implements SnapshotPreview1.EnvironSizesGet
func (a *wasiAPI) EnvironSizesGet(ctx wasm.Module, resultEnvironc uint32, resultEnvironBufSize uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	if !ctx.Memory().WriteUint32Le(resultEnvironc, uint32(len(cfg.environ.nullTerminatedValues))) {
		return wasi.ErrnoFault
	}
	if !ctx.Memory().WriteUint32Le(resultEnvironBufSize, cfg.environ.totalBufSize) {
		return wasi.ErrnoFault
	}

	return wasi.ErrnoSuccess
}

// ClockResGet implements SnapshotPreview1.ClockResGet
func (a *wasiAPI) ClockResGet(ctx wasm.Module, id uint32, resultResolution uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// ClockTimeGet implements SnapshotPreview1.ClockTimeGet
func (a *wasiAPI) ClockTimeGet(ctx wasm.Module, id uint32, precision uint64, resultTimestamp uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	// TODO: id and precision are currently ignored.
	if !ctx.Memory().WriteUint64Le(resultTimestamp, cfg.timeNowUnixNano()) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// FdAdvise implements SnapshotPreview1.FdAdvise
func (a *wasiAPI) FdAdvise(ctx wasm.Module, fd uint32, offset, len uint64, resultAdvice uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdAllocate implements SnapshotPreview1.FdAllocate
func (a *wasiAPI) FdAllocate(ctx wasm.Module, fd uint32, offset, len uint64) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdClose implements SnapshotPreview1.FdClose
func (a *wasiAPI) FdClose(ctx wasm.Module, fd uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	f, ok := cfg.opened[fd]
	if !ok {
		return wasi.ErrnoBadf
	}

	if f.file != nil {
		f.file.Close()
	}

	delete(cfg.opened, fd)

	return wasi.ErrnoSuccess
}

// FdDatasync implements SnapshotPreview1.FdDatasync
func (a *wasiAPI) FdDatasync(ctx wasm.Module, fd uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFdstatGet implements SnapshotPreview1.FdFdstatGet
// TODO: Currently FdFdstatget implements nothing except returning fake fs_right_inheriting
func (a *wasiAPI) FdFdstatGet(ctx wasm.Module, fd uint32, resultStat uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	if _, ok := cfg.opened[fd]; !ok {
		return wasi.ErrnoBadf
	}
	if !ctx.Memory().WriteUint64Le(resultStat+16, rightFDRead|rightFDWrite) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// FdPrestatGet implements SnapshotPreview1.FdPrestatGet
func (a *wasiAPI) FdPrestatGet(ctx wasm.Module, fd uint32, resultPrestat uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	entry, ok := cfg.opened[fd]
	if !ok || entry.preopenPath == "" {
		return wasi.ErrnoBadf
	}

	// Zero-value 8-bit tag, and 3-byte zero-value paddings, which is uint32le(0) in short.
	if !ctx.Memory().WriteUint32Le(resultPrestat, uint32(0)) {
		return wasi.ErrnoFault
	}
	// Write the length of the directory name at offset 4.
	if !ctx.Memory().WriteUint32Le(resultPrestat+4, uint32(len(entry.preopenPath))) {
		return wasi.ErrnoFault
	}

	return wasi.ErrnoSuccess
}

// FdFdstatSetFlags implements SnapshotPreview1.FdFdstatSetFlags
func (a *wasiAPI) FdFdstatSetFlags(ctx wasm.Module, fd uint32, flags uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFdstatSetRights implements SnapshotPreview1.FdFdstatSetRights
func (a *wasiAPI) FdFdstatSetRights(ctx wasm.Module, fd uint32, fsRightsBase, fsRightsInheriting uint64) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatGet implements SnapshotPreview1.FdFilestatGet
func (a *wasiAPI) FdFilestatGet(ctx wasm.Module, fd uint32, resultBuf uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatSetSize implements SnapshotPreview1.FdFilestatSetSize
func (a *wasiAPI) FdFilestatSetSize(ctx wasm.Module, fd uint32, size uint64) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatSetTimes implements SnapshotPreview1.FdFilestatSetTimes
func (a *wasiAPI) FdFilestatSetTimes(ctx wasm.Module, fd uint32, atim, mtim uint64, fstFlags uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdPread implements SnapshotPreview1.FdPread
func (a *wasiAPI) FdPread(ctx wasm.Module, fd, iovs, iovsCount uint32, offset uint64, resultNread uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdPrestatDirName implements SnapshotPreview1.FdPrestatDirName
func (a *wasiAPI) FdPrestatDirName(ctx wasm.Module, fd uint32, pathPtr uint32, pathLen uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	f, ok := cfg.opened[fd]
	if !ok || f.preopenPath == "" {
		return wasi.ErrnoBadf
	}

	// Some runtimes may have another semantics. See internal/wasi/RATIONALE.md
	if uint32(len(f.preopenPath)) < pathLen {
		return wasi.ErrnoNametoolong
	}

	if !ctx.Memory().Write(pathPtr, []byte(f.preopenPath)[:pathLen]) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// FdPwrite implements SnapshotPreview1.FdPwrite
func (a *wasiAPI) FdPwrite(ctx wasm.Module, fd, iovs, iovsCount uint32, offset uint64, resultNwritten uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdRead implements SnapshotPreview1.FdRead
func (a *wasiAPI) FdRead(ctx wasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	f, ok := cfg.opened[fd]
	if !ok {
		return wasi.ErrnoBadf
	}

	var nread uint32
	for i := uint32(0); i < iovsCount; i++ {
		iovPtr := iovs + i*8
		offset, ok := ctx.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return wasi.ErrnoFault
		}
		l, ok := ctx.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return wasi.ErrnoFault
		}
		b, ok := ctx.Memory().Read(offset, l)
		if !ok {
			return wasi.ErrnoFault
		}
		n, err := f.file.Read(b)
		nread += uint32(n)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return wasi.ErrnoIo
		}
	}
	if !ctx.Memory().WriteUint32Le(resultSize, nread) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// FdReaddir implements SnapshotPreview1.FdReaddir
func (a *wasiAPI) FdReaddir(ctx wasm.Module, fd, buf, bufLen uint32, cookie uint64, resultBufused uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdRenumber implements SnapshotPreview1.FdRenumber
func (a *wasiAPI) FdRenumber(ctx wasm.Module, fd, to uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdSeek implements SnapshotPreview1.FdSeek
func (a *wasiAPI) FdSeek(ctx wasm.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	f, ok := cfg.opened[fd]
	if !ok || f.file == nil {
		return wasi.ErrnoBadf
	}
	seeker, ok := f.file.(io.Seeker)
	if !ok {
		return wasi.ErrnoBadf
	}

	if whence > io.SeekEnd /* exceeds the largest valid whence */ {
		return wasi.ErrnoInval
	}
	newOffst, err := seeker.Seek(int64(offset), int(whence))
	if err != nil {
		return wasi.ErrnoIo
	}

	if !ctx.Memory().WriteUint32Le(resultNewoffset, uint32(newOffst)) {
		return wasi.ErrnoFault
	}

	return wasi.ErrnoSuccess
}

// FdSync implements SnapshotPreview1.FdSync
func (a *wasiAPI) FdSync(ctx wasm.Module, fd uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdTell implements SnapshotPreview1.FdTell
func (a *wasiAPI) FdTell(ctx wasm.Module, fd, resultOffset uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// FdWrite implements SnapshotPreview1.FdWrite
func (a *wasiAPI) FdWrite(ctx wasm.Module, fd, iovs, iovsCount, resultSize uint32) wasi.Errno {
	cfg := a.config(ctx.Context())

	f, ok := cfg.opened[fd]
	if !ok {
		return wasi.ErrnoBadf
	}
	writer, ok := f.file.(io.Writer)
	if !ok {
		return wasi.ErrnoBadf
	}

	var nwritten uint32
	for i := uint32(0); i < iovsCount; i++ {
		iovPtr := iovs + i*8
		offset, ok := ctx.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return wasi.ErrnoFault
		}
		l, ok := ctx.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return wasi.ErrnoFault
		}
		b, ok := ctx.Memory().Read(offset, l)
		if !ok {
			return wasi.ErrnoFault
		}
		n, err := writer.Write(b)
		if err != nil {
			return wasi.ErrnoIo
		}
		nwritten += uint32(n)
	}
	if !ctx.Memory().WriteUint32Le(resultSize, nwritten) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// PathCreateDirectory implements SnapshotPreview1.PathCreateDirectory
func (a *wasiAPI) PathCreateDirectory(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathFilestatGet implements SnapshotPreview1.PathFilestatGet
func (a *wasiAPI) PathFilestatGet(ctx wasm.Module, fd, flags, path, pathLen, resultBuf uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathFilestatSetTimes implements SnapshotPreview1.PathFilestatSetTimes
func (a *wasiAPI) PathFilestatSetTimes(ctx wasm.Module, fd, flags, path, pathLen uint32, atim, mtime uint64, fstFlags uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathLink implements SnapshotPreview1.PathLink
func (a *wasiAPI) PathLink(ctx wasm.Module, oldFd, oldFlags, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

const (
	// WASI open flags
	oflagCreate = 1 << iota
	oflagDir
	oflagExclusive
	oflagTrunc

	// WASI FS rights
	rightFDRead  = 1 << iota
	rightFDWrite = 0x200
)

func posixOpenFlags(oFlags uint32, fsRights uint64) (pFlags int) {
	// TODO: handle dirflags, which decides whether to follow symbolic links or not,
	//       by O_NOFOLLOW. Note O_NOFOLLOW doesn't exist on Windows.
	if fsRights&rightFDWrite != 0 {
		if fsRights&rightFDRead != 0 {
			pFlags |= os.O_RDWR
		} else {
			pFlags |= os.O_WRONLY
		}
	}
	if oFlags&oflagCreate != 0 {
		pFlags |= os.O_CREATE
	}
	if oFlags&oflagExclusive != 0 {
		pFlags |= os.O_EXCL
	}
	if oFlags&oflagTrunc != 0 {
		pFlags |= os.O_TRUNC
	}
	return
}

// PathOpen implements SnapshotPreview1.PathOpen
func (a *wasiAPI) PathOpen(ctx wasm.Module, fd, dirflags, pathPtr, pathLen, oflags uint32, fsRightsBase,
	fsRightsInheriting uint64, fdflags, resultOpenedFd uint32) (errno wasi.Errno) {
	cfg := a.config(ctx.Context())

	dir, ok := cfg.opened[fd]
	if !ok || dir.fileSys == nil {
		return wasi.ErrnoBadf
	}

	b, ok := ctx.Memory().Read(pathPtr, pathLen)
	if !ok {
		return wasi.ErrnoFault
	}

	// Clean the path because fs.FS.Open and OpenFileFS.OpenFile need path satisfying `fs.ValidPath(path)`.
	// See fs.FS.Open, wasi.OpenFileFS.OpenFile, fs.ValidPath.
	pathName := path.Clean(dir.path + "/" + string(b))
	var f fs.File
	var err error
	if openFileFS, ok := dir.fileSys.(wasi.OpenFileFS); ok {
		f, err = openFileFS.OpenFile(pathName, posixOpenFlags(oflags, fsRightsBase), fs.FileMode(0644))
	} else {
		// Pure read-only fs.FS. We do not check oFlags here, but non-read operations will fail later in each API.
		f, err = dir.fileSys.Open(pathName)
	}
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return wasi.ErrnoNoent
		case errors.Is(err, fs.ErrExist):
			return wasi.ErrnoExist
		default:
			return wasi.ErrnoIo
		}
	}

	// when ofagDir is set, the opened file must be a directory.
	if oflags&oflagDir != 0 {
		stat, err := f.Stat()
		if err != nil || !stat.IsDir() {
			return wasi.ErrnoNotdir
		}
	}

	newFD, err := a.randUnusedFD(cfg)
	if err != nil {
		return wasi.ErrnoIo
	}

	cfg.opened[newFD] = fileEntry{
		path:    pathName,
		fileSys: dir.fileSys,
		file:    f,
	}

	if !ctx.Memory().WriteUint32Le(resultOpenedFd, newFD) {
		return wasi.ErrnoFault
	}
	return wasi.ErrnoSuccess
}

// PathReadlink implements SnapshotPreview1.PathReadlink
func (a *wasiAPI) PathReadlink(ctx wasm.Module, fd, path, pathLen, buf, bufLen, resultBufused uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathRemoveDirectory implements SnapshotPreview1.PathRemoveDirectory
func (a *wasiAPI) PathRemoveDirectory(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathRename implements SnapshotPreview1.PathRename
func (a *wasiAPI) PathRename(ctx wasm.Module, fd, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathSymlink implements SnapshotPreview1.PathSymlink
func (a *wasiAPI) PathSymlink(ctx wasm.Module, oldPath, oldPathLen, fd, newPath, newPathLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PathUnlinkFile implements SnapshotPreview1.PathUnlinkFile
func (a *wasiAPI) PathUnlinkFile(ctx wasm.Module, fd, path, pathLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// PollOneoff implements SnapshotPreview1.PollOneoff
func (a *wasiAPI) PollOneoff(ctx wasm.Module, in, out, nsubscriptions, resultNevents uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// ProcExit implements SnapshotPreview1.ProcExit
func (a *wasiAPI) ProcExit(exitCode uint32) {
	// Panic in a host function is caught by the engines, and the value of the panic is returned as the error of the CallFunction.
	// See the document of SnapshotPreview1.ProcExit.
	panic(wasi.ExitCode(exitCode))
}

// ProcRaise implements SnapshotPreview1.ProcRaise
func (a *wasiAPI) ProcRaise(ctx wasm.Module, sig uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// SchedYield implements SnapshotPreview1.SchedYield
func (a *wasiAPI) SchedYield(ctx wasm.Module) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// RandomGet implements SnapshotPreview1.RandomGet
func (a *wasiAPI) RandomGet(ctx wasm.Module, buf uint32, bufLen uint32) (errno wasi.Errno) {
	cfg := a.config(ctx.Context())

	randomBytes := make([]byte, bufLen)
	err := cfg.randSource(randomBytes)
	if err != nil {
		// TODO: handle different errors that syscal to entropy source can return
		return wasi.ErrnoIo
	}

	if !ctx.Memory().Write(buf, randomBytes) {
		return wasi.ErrnoFault
	}

	return wasi.ErrnoSuccess
}

// SockRecv implements SnapshotPreview1.SockRecv
func (a *wasiAPI) SockRecv(ctx wasm.Module, fd, riData, riDataCount, riFlags, resultRoDataLen, resultRoFlags uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// SockSend implements SnapshotPreview1.SockSend
func (a *wasiAPI) SockSend(ctx wasm.Module, fd, siData, siDataCount, siFlags, resultSoDataLen uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// SockShutdown implements SnapshotPreview1.SockShutdown
func (a *wasiAPI) SockShutdown(ctx wasm.Module, fd, how uint32) wasi.Errno {
	return wasi.ErrnoNosys // stubbed for GrainLang per #271
}

// fileEntry is an entry of the opened file descriptors table.
type fileEntry struct {
	// If this entry is a pre-opend directory, preopenPath is the path to this directory
	// in the WASI environment, or "." if this directory is opened as a working directory.
	// Empty when this entry is not a pre-opened directory.
	preopenPath string
	// File path relative to the root of `fileSys`.
	// Empty if this opened entry has no path, such as Stdio.
	path string
	// fs.FS instance that this file belongs to.
	// nil if this opened entry has no FS, such as Stdio.
	fileSys fs.FS
	// Opened fs.File instance.
	file fs.File
}

// ConfigContextKey indicates a context.Context includes an overriding Config.
type ConfigContextKey struct{}

type Config struct {
	args *nullTerminatedStrings
	// environ stores each environment variable in the form of "key=value",
	// which is both convenient for the implementation of environ_get and matches os.Environ
	environ *nullTerminatedStrings
	opened  map[uint32]fileEntry
	// timeNowUnixNano is mutable for testing
	timeNowUnixNano func() uint64
	randSource      func([]byte) error
}

// FD number constants of standard input / output / error.
const (
	stdinFD = iota
	stdoutFD
	stderrFD
)

func (c *Config) Stdin(reader io.Reader) {
	c.opened[stdinFD] = fileEntry{file: &readerWriterFile{Reader: reader}}
}

func (c *Config) Stdout(writer io.Writer) {
	c.opened[stdoutFD] = fileEntry{file: &readerWriterFile{Writer: writer}}
}

func (c *Config) Stderr(writer io.Writer) {
	c.opened[stderrFD] = fileEntry{file: &readerWriterFile{Writer: writer}}
}

// Args returns an option to give a command-line arguments in SnapshotPreview1 or errs if the inputs are too large.
//
// Note: The only reason to set this is to control what's written by SnapshotPreview1.ArgsSizesGet and SnapshotPreview1.ArgsGet
// Note: While similar in structure to os.Args, this controls what's visible in Wasm (ex the WASI function "_start").
func (c *Config) Args(args ...string) error {
	wasiStrings, err := newNullTerminatedStrings(math.MaxUint32, "arg", args...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return err
	}
	c.args = wasiStrings
	return nil
}

// Environ returns an option to set environment variables in SnapshotPreview1.
// Environ returns an error if the input contains a string not joined with `=`, or if the inputs are too large.
//  * environ: environment variables in the same format as that of `os.Environ`, where key/value pairs are joined with `=`.
// See os.Environ
//
// Note: Implicit environment variable propagation into WASI is intentionally not done.
// Note: The only reason to set this is to control what's written by SnapshotPreview1.EnvironSizesGet and SnapshotPreview1.EnvironGet
// Note: While similar in structure to os.Environ, this controls what's visible in Wasm (ex the WASI function "_start").
func (c *Config) Environ(environ ...string) error {
	for i, env := range environ {
		if !strings.Contains(env, "=") {
			return fmt.Errorf("environ[%d] is not joined with '='", i)
		}
	}
	wasiStrings, err := newNullTerminatedStrings(math.MaxUint32, "environ", environ...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return err
	}
	c.environ = wasiStrings
	return nil
}

func (c *Config) Preopen(dir string, fileSys fs.FS) {
	rootFile, err := fileSys.Open(".")
	if err != nil {
		// Opening "." should always succeed on a fs.FS correctly implemented. Panic instead of returning error.
		panic(fmt.Errorf("failed to open '.' for pre-opened FS: %w", err))
	}
	c.opened[uint32(len(c.opened))] = fileEntry{
		preopenPath: dir,
		path:        ".",
		fileSys:     fileSys,
		file:        rootFile,
	}
}

// NewConfig sets configuration defaults
func NewConfig() *Config {
	return &Config{
		args:    &nullTerminatedStrings{},
		environ: &nullTerminatedStrings{},
		opened: map[uint32]fileEntry{
			stdinFD:  {file: &readerWriterFile{}},
			stdoutFD: {file: &readerWriterFile{}},
			stderrFD: {file: &readerWriterFile{}},
		},
		timeNowUnixNano: func() uint64 {
			return uint64(time.Now().UnixNano())
		},
		randSource: func(p []byte) error {
			_, err := crand.Read(p)
			return err
		},
	}
}

// NewAPI is exported for benchmarks
func NewAPI(config *Config) *wasiAPI {
	return &wasiAPI{cfg: config} // Safe copy
}

// config returns a potentially overridden Config.
func (a *wasiAPI) config(ctx context.Context) *Config {
	if cfg := ctx.Value(ConfigContextKey{}); cfg != nil {
		return cfg.(*Config)
	}
	return a.cfg
}

func (a *wasiAPI) randUnusedFD(config *Config) (uint32, error) {
	rand := make([]byte, 4)
	err := config.randSource(rand)
	if err != nil {
		return 0, err
	}
	// fd is actually a signed int32, and must be a positive number.
	fd := binary.LittleEndian.Uint32(rand) % (1 << 31)
	for {
		if _, ok := config.opened[fd]; !ok {
			return fd, nil
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func ValidateWASICommand(module *internalwasm.Module, moduleName string) error {
	if start, err := requireExport(module, moduleName, FunctionStart, internalwasm.ExternTypeFunc); err != nil {
		return err
	} else {
		// TODO: this should be verified during decode so that errors have the correct source positions
		ft := module.TypeOfFunction(start.Index)
		if ft == nil {
			return fmt.Errorf("module[%s] function[%s] has an invalid type", moduleName, FunctionStart)
		}
		if len(ft.Params) > 0 || len(ft.Results) > 0 {
			return fmt.Errorf("module[%s] function[%s] must have an empty (nullary) signature: %s", moduleName, FunctionStart, ft.String())
		}
	}
	if _, err := requireExport(module, moduleName, FunctionInitialize, internalwasm.ExternTypeFunc); err == nil {
		return fmt.Errorf("module[%s] must not export func[%s]", moduleName, FunctionInitialize)
	}
	if _, err := requireExport(module, moduleName, "memory", internalwasm.ExternTypeMemory); err != nil {
		return err
	}
	// TODO: the spec also requires export of "__indirect_function_table", but we aren't enforcing it, and doing so
	// would break existing users of TinyGo who aren't exporting that. We could possibly scan to see if it is every used.
	return nil
}

func requireExport(module *internalwasm.Module, moduleName string, exportName string, kind internalwasm.ExternType) (*internalwasm.Export, error) {
	exp, ok := module.ExportSection[exportName]
	if !ok || exp.Type != kind {
		return nil, fmt.Errorf("module[%s] does not export %s[%s]", moduleName, internalwasm.ExternTypeName(kind), exportName)
	}
	return exp, nil
}
