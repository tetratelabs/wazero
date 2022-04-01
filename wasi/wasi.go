package wasi

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	// ModuleSnapshotPreview1 is the module name WASI functions are exported into
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
	ModuleSnapshotPreview1 = "wasi_snapshot_preview1"
)

// InstantiateSnapshotPreview1 instantiates ModuleSnapshotPreview1, so that other modules can import them.
//
// Ex. After you configure like this, other modules can import functions like "wasi_snapshot_preview1" "fd_write".
//	wm, _ := wasi.InstantiateSnapshotPreview1(r)
//	defer wm.Close()
//
// Note: All WASI functions return a single Errno result, ErrnoSuccess on success.
func InstantiateSnapshotPreview1(r wazero.Runtime) (api.Module, error) {
	_, fns := snapshotPreview1Functions()
	return r.NewModuleBuilder(ModuleSnapshotPreview1).ExportFunctions(fns).Instantiate()
}

const (
	// functionStart is the name of the nullary function a module must export if it is a WASI Command Module.
	//
	// Note: When this is exported functionInitialize must not be.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	functionStart = "_start" // nolint

	// functionInitialize is the name of the nullary function a module must export if it is a WASI Reactor Module.
	//
	// Note: When this is exported functionStart must not be.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
	functionInitialize = "_initialize" // nolint

	// functionArgsGet reads command-line argument data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	functionArgsGet = "args_get"

	// importArgsGet is the WebAssembly 1.0 (20191205) Text format import of functionArgsGet.
	importArgsGet = `(import "wasi_snapshot_preview1" "args_get"
    (func $wasi.args_get (param $argv i32) (param $argv_buf i32) (result (;errno;) i32)))`

	// functionArgsSizesGet returns command-line argument data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	functionArgsSizesGet = "args_sizes_get"

	// importArgsSizesGet is the WebAssembly 1.0 (20191205) Text format import of functionArgsSizesGet.
	importArgsSizesGet = `(import "wasi_snapshot_preview1" "args_sizes_get"
    (func $wasi.args_sizes_get (param $result.argc i32) (param $result.argv_buf_size i32) (result (;errno;) i32)))`

	// functionEnvironGet reads environment variable data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	functionEnvironGet = "environ_get"

	// importEnvironGet is the WebAssembly 1.0 (20191205) Text format import of functionEnvironGet.
	importEnvironGet = `(import "wasi_snapshot_preview1" "environ_get"
    (func $wasi.environ_get (param $environ i32) (param $environ_buf i32) (result (;errno;) i32)))`

	// functionEnvironSizesGet returns environment variable data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	functionEnvironSizesGet = "environ_sizes_get"

	// importEnvironSizesGet is the WebAssembly 1.0 (20191205) Text format import of functionEnvironSizesGet.
	importEnvironSizesGet = `(import "wasi_snapshot_preview1" "environ_sizes_get"
    (func $wasi.environ_sizes_get (param $result.environc i32) (param $result.environBufSize i32) (result (;errno;) i32)))`

	// functionClockResGet returns the resolution of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	functionClockResGet = "clock_res_get"

	// importClockResGet is the WebAssembly 1.0 (20191205) Text format import of functionClockResGet.
	importClockResGet = `(import "wasi_snapshot_preview1" "clock_res_get"
    (func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))`

	// functionClockTimeGet returns the time value of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	functionClockTimeGet = "clock_time_get"

	// importClockTimeGet is the WebAssembly 1.0 (20191205) Text format import of functionClockTimeGet.
	importClockTimeGet = `(import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))`

	// functionFdAdvise provides file advisory information on a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_advisefd-fd-offset-filesize-len-filesize-advice-advice---errno
	functionFdAdvise = "fd_advise"

	// importFdAdvise is the WebAssembly 1.0 (20191205) Text format import of functionFdAdvise.
	importFdAdvise = `(import "wasi_snapshot_preview1" "fd_advise"
    (func $wasi.fd_advise (param $fd i32) (param $offset i64) (param $len i64) (param $result.advice i32) (result (;errno;) i32)))`

	// functionFdAllocate forces the allocation of space in a file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_allocatefd-fd-offset-filesize-len-filesize---errno
	functionFdAllocate = "fd_allocate"

	// importFdAllocate is the WebAssembly 1.0 (20191205) Text format import of functionFdAllocate.
	importFdAllocate = `(import "wasi_snapshot_preview1" "fd_allocate"
    (func $wasi.fd_allocate (param $fd i32) (param $offset i64) (param $len i64) (result (;errno;) i32)))`

	// functionFdClose closes a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_close
	functionFdClose = "fd_close"

	// importFdClose is the WebAssembly 1.0 (20191205) Text format import of functionFdClose.
	importFdClose = `(import "wasi_snapshot_preview1" "fd_close"
    (func $wasi.fd_close (param $fd i32) (result (;errno;) i32)))`

	// functionFdDatasync synchronizes the data of a file to disk.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_close
	functionFdDatasync = "fd_datasync"

	// importFdDatasync is the WebAssembly 1.0 (20191205) Text format import of functionFdDatasync.
	importFdDatasync = `(import "wasi_snapshot_preview1" "fd_datasync"
    (func $wasi.fd_datasync (param $fd i32) (result (;errno;) i32)))`

	// functionFdFdstatGet gets the attributes of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_getfd-fd---errno-fdstat
	functionFdFdstatGet = "fd_fdstat_get"

	// importFdFdstatGet is the WebAssembly 1.0 (20191205) Text format import of functionFdFdstatGet.
	importFdFdstatGet = `(import "wasi_snapshot_preview1" "fd_fdstat_get"
    (func $wasi.fd_fdstat_get (param $fd i32) (param $result.stat i32) (result (;errno;) i32)))`  //nolint

	// functionFdFdstatSetFlags adjusts the flags associated with a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_flagsfd-fd-flags-fdflags---errno
	functionFdFdstatSetFlags = "fd_fdstat_set_flags"

	// importFdFdstatSetFlags is the WebAssembly 1.0 (20191205) Text format import of functionFdFdstatSetFlags.
	importFdFdstatSetFlags = `(import "wasi_snapshot_preview1" "fd_fdstat_set_flags"
    (func $wasi.fd_fdstat_set_flags (param $fd i32) (param $flags i32) (result (;errno;) i32)))`

	// functionFdFdstatSetRights adjusts the rights associated with a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_fdstat_set_rightsfd-fd-fs_rights_base-rights-fs_rights_inheriting-rights---errno
	functionFdFdstatSetRights = "fd_fdstat_set_rights"

	// importFdFdstatSetRights is the WebAssembly 1.0 (20191205) Text format import of functionFdFdstatSetRights.
	importFdFdstatSetRights = `(import "wasi_snapshot_preview1" "fd_fdstat_set_rights"
    (func $wasi.fd_fdstat_set_rights (param $fd i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (result (;errno;) i32)))`

	// functionFdFilestatGet returns the attributes of an open file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_getfd-fd---errno-filestat
	functionFdFilestatGet = "fd_filestat_get"

	// importFdFilestatGet is the WebAssembly 1.0 (20191205) Text format import of functionFdFilestatGet.
	importFdFilestatGet = `(import "wasi_snapshot_preview1" "fd_filestat_get"
    (func $wasi.fd_filestat_get (param $fd i32) (param $result.buf i32) (result (;errno;) i32)))`

	// functionFdFilestatSetSize adjusts the size of an open file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_set_sizefd-fd-size-filesize---errno
	functionFdFilestatSetSize = "fd_filestat_set_size"

	// importFdFilestatSetSize is the WebAssembly 1.0 (20191205) Text format import of functionFdFilestatSetSize.
	importFdFilestatSetSize = `(import "wasi_snapshot_preview1" "fd_filestat_set_size"
    (func $wasi.fd_filestat_set_size (param $fd i32) (param $size i64) (result (;errno;) i32)))`

	// functionFdFilestatSetTimes adjusts the times of an open file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_filestat_set_timesfd-fd-atim-timestamp-mtim-timestamp-fst_flags-fstflags---errno
	functionFdFilestatSetTimes = "fd_filestat_set_times"

	// importFdFilestatSetTimes is the WebAssembly 1.0 (20191205) Text format import of functionFdFilestatSetTimes.
	importFdFilestatSetTimes = `(import "wasi_snapshot_preview1" "fd_filestat_set_times"
    (func $wasi.fd_filestat_set_times (param $fd i32) (param $atim i64) (param $mtim i64) (param $fst_flags i32) (result (;errno;) i32)))`

	// functionFdPread reads from a file descriptor, without using and updating the file descriptor's offset.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_preadfd-fd-iovs-iovec_array-offset-filesize---errno-size
	functionFdPread = "fd_pread"

	// importFdPread is the WebAssembly 1.0 (20191205) Text format import of functionFdPread.
	importFdPread = `(import "wasi_snapshot_preview1" "fd_pread"
    (func $wasi.fd_pread (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $offset i64) (param $result.nread i32) (result (;errno;) i32)))`

	// functionFdPrestatGet returns the prestat data of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_get
	functionFdPrestatGet = "fd_prestat_get"

	// importFdPrestatGet is the WebAssembly 1.0 (20191205) Text format import of functionFdPrestatGet.
	importFdPrestatGet = `(import "wasi_snapshot_preview1" "fd_prestat_get"
    (func $wasi.fd_prestat_get (param $fd i32) (param $result.prestat i32) (result (;errno;) i32)))`

	// functionFdPrestatDirName returns the path of the pre-opened directory of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_dir_name
	functionFdPrestatDirName = "fd_prestat_dir_name"

	// importFdPrestatDirName is the WebAssembly 1.0 (20191205) Text format import of functionFdPrestatDirName.
	importFdPrestatDirName = `(import "wasi_snapshot_preview1" "fd_prestat_dir_name"
    (func $wasi.fd_prestat_dir_name (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// functionFdPwrite writes to a file descriptor, without using and updating the file descriptor's offset.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_pwritefd-fd-iovs-ciovec_array-offset-filesize---errno-size
	functionFdPwrite = "fd_pwrite"

	// importFdPwrite is the WebAssembly 1.0 (20191205) Text format import of functionFdPwrite.
	importFdPwrite = `(import "wasi_snapshot_preview1" "fd_pwrite"
    (func $wasi.fd_pwrite (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $offset i64) (param $result.nwritten i32) (result (;errno;) i32)))`

	// functionFdRead read bytes from a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_read
	functionFdRead = "fd_read"

	// importFdRead is the WebAssembly 1.0 (20191205) Text format import of functionFdRead.
	importFdRead = `(import "wasi_snapshot_preview1" "fd_read"
    (func $wasi.fd_read (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))`

	// functionFdReaddir reads directory entries from a directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_readdirfd-fd-buf-pointeru8-buf_len-size-cookie-dircookie---errno-size
	functionFdReaddir = "fd_readdir"

	// importFdReaddir is the WebAssembly 1.0 (20191205) Text format import of functionFdReaddir.
	importFdReaddir = `(import "wasi_snapshot_preview1" "fd_readdir"
    (func $wasi.fd_readdir (param $fd i32) (param $buf i32) (param $buf_len i32) (param $cookie i64) (param $result.bufused i32) (result (;errno;) i32)))`

	// functionFdRenumber atomically replaces a file descriptor by renumbering another file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_renumberfd-fd-to-fd---errno
	functionFdRenumber = "fd_renumber"

	// importFdRenumber is the WebAssembly 1.0 (20191205) Text format import of functionFdRenumber.
	importFdRenumber = `(import "wasi_snapshot_preview1" "fd_renumber"
    (func $wasi.fd_renumber (param $fd i32) (param $to i32) (result (;errno;) i32)))`

	// functionFdSeek moves the offset of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_seekfd-fd-offset-filedelta-whence-whence---errno-filesize
	functionFdSeek = "fd_seek"

	// importFdSeek is the WebAssembly 1.0 (20191205) Text format import of functionFdSeek.
	importFdSeek = `(import "wasi_snapshot_preview1" "fd_seek"
    (func $wasi.fd_seek (param $fd i32) (param $offset i64) (param $whence i32) (param $result.newoffset i32) (result (;errno;) i32)))`

	// functionFdSync synchronizes the data and metadata of a file to disk.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_syncfd-fd---errno
	functionFdSync = "fd_sync"

	// importFdSync is the WebAssembly 1.0 (20191205) Text format import of functionFdSync.
	importFdSync = `(import "wasi_snapshot_preview1" "fd_sync"
    (func $wasi.fd_sync (param $fd i32) (result (;errno;) i32)))`

	// functionFdTell returns the current offset of a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-fd_tellfd-fd---errno-filesize
	functionFdTell = "fd_tell"

	// importFdTell is the WebAssembly 1.0 (20191205) Text format import of functionFdTell.
	importFdTell = `(import "wasi_snapshot_preview1" "fd_tell"
    (func $wasi.fd_tell (param $fd i32) (param $result.offset i32) (result (;errno;) i32)))`

	// functionFdWrite write bytes to a file descriptor.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_write
	functionFdWrite = "fd_write"

	// importFdWrite is the WebAssembly 1.0 (20191205) Text format import of functionFdWrite.
	importFdWrite = `(import "wasi_snapshot_preview1" "fd_write"
    (func $wasi.fd_write (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))`

	// functionPathCreateDirectory creates a directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_create_directoryfd-fd-path-string---errno
	functionPathCreateDirectory = "path_create_directory"

	// importPathCreateDirectory is the WebAssembly 1.0 (20191205) Text format import of functionPathCreateDirectory.
	importPathCreateDirectory = `(import "wasi_snapshot_preview1" "path_create_directory"
    (func $wasi.path_create_directory (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// functionPathFilestatGet returns the attributes of a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_getfd-fd-flags-lookupflags-path-string---errno-filestat
	functionPathFilestatGet = "path_filestat_get"

	// importPathFilestatGet is the WebAssembly 1.0 (20191205) Text format import of functionPathFilestatGet.
	importPathFilestatGet = `(import "wasi_snapshot_preview1" "path_filestat_get"
    (func $wasi.path_filestat_get (param $fd i32) (param $flags i32) (param $path i32) (param $path_len i32) (param $result.buf i32) (result (;errno;) i32)))`

	// functionPathFilestatSetTimes adjusts the timestamps of a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_filestat_set_timesfd-fd-flags-lookupflags-path-string-atim-timestamp-mtim-timestamp-fst_flags-fstflags---errno
	functionPathFilestatSetTimes = "path_filestat_set_times"

	// importPathFilestatSetTimes is the WebAssembly 1.0 (20191205) Text format import of functionPathFilestatSetTimes.
	importPathFilestatSetTimes = `(import "wasi_snapshot_preview1" "path_filestat_set_times"
    (func $wasi.path_filestat_set_times (param $fd i32) (param $flags i32) (param $path i32) (param $path_len i32) (param $atim i64) (param $mtim i64) (param $fst_flags i32) (result (;errno;) i32)))`

	// functionPathLink adjusts the timestamps of a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#path_link
	functionPathLink = "path_link"

	// importPathLink is the WebAssembly 1.0 (20191205) Text format import of functionPathLink.
	importPathLink = `(import "wasi_snapshot_preview1" "path_link"
    (func $wasi.path_link (param $old_fd i32) (param $old_flags i32) (param $old_path i32) (param $old_path_len i32) (param $new_fd i32) (param $new_path i32) (param $new_path_len i32) (result (;errno;) i32)))`

	// functionPathOpen opens a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_openfd-fd-dirflags-lookupflags-path-string-oflags-oflags-fs_rights_base-rights-fs_rights_inheriting-rights-fdflags-fdflags---errno-fd
	functionPathOpen = "path_open"

	// importPathOpen is the WebAssembly 1.0 (20191205) Text format import of functionPathOpen.
	importPathOpen = `(import "wasi_snapshot_preview1" "path_open"
    (func $wasi.path_open (param $fd i32) (param $dirflags i32) (param $path i32) (param $path_len i32) (param $oflags i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (param $fdflags i32) (param $result.opened_fd i32) (result (;errno;) i32)))`

	// functionPathReadlink reads the contents of a symbolic link.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_readlinkfd-fd-path-string-buf-pointeru8-buf_len-size---errno-size
	functionPathReadlink = "path_readlink"

	// importPathReadlink is the WebAssembly 1.0 (20191205) Text format import of functionPathReadlink.
	importPathReadlink = `(import "wasi_snapshot_preview1" "path_readlink"
    (func $wasi.path_readlink (param $fd i32) (param $path i32) (param $path_len i32) (param $buf i32) (param $buf_len i32) (param $result.bufused i32) (result (;errno;) i32)))`

	// functionPathRemoveDirectory removes a directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_remove_directoryfd-fd-path-string---errno
	functionPathRemoveDirectory = "path_remove_directory"

	// importPathRemoveDirectory is the WebAssembly 1.0 (20191205) Text format import of functionPathRemoveDirectory.
	importPathRemoveDirectory = `(import "wasi_snapshot_preview1" "path_remove_directory"
    (func $wasi.path_remove_directory (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// functionPathRename renames a file or directory.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_renamefd-fd-old_path-string-new_fd-fd-new_path-string---errno
	functionPathRename = "path_rename"

	// importPathRename is the WebAssembly 1.0 (20191205) Text format import of functionPathRename.
	importPathRename = `(import "wasi_snapshot_preview1" "path_rename"
    (func $wasi.path_rename (param $fd i32) (param $old_path i32) (param $old_path_len i32) (param $new_fd i32) (param $new_path i32) (param $new_path_len i32) (result (;errno;) i32)))`

	// functionPathSymlink creates a symbolic link.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#path_symlink
	functionPathSymlink = "path_symlink"

	// importPathSymlink is the WebAssembly 1.0 (20191205) Text format import of functionPathSymlink.
	importPathSymlink = `(import "wasi_snapshot_preview1" "path_symlink"
    (func $wasi.path_symlink (param $old_path i32) (param $old_path_len i32) (param $fd i32) (param $new_path i32) (param $new_path_len i32) (result (;errno;) i32)))`

	// functionPathUnlinkFile unlinks a file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-path_unlink_filefd-fd-path-string---errno
	functionPathUnlinkFile = "path_unlink_file"

	// importPathUnlinkFile is the WebAssembly 1.0 (20191205) Text format import of functionPathUnlinkFile.
	importPathUnlinkFile = `(import "wasi_snapshot_preview1" "path_unlink_file"
    (func $wasi.path_unlink_file (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))`

	// functionPollOneoff unlinks a file.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-poll_oneoffin-constpointersubscription-out-pointerevent-nsubscriptions-size---errno-size
	functionPollOneoff = "poll_oneoff"

	// importPollOneoff is the WebAssembly 1.0 (20191205) Text format import of functionPollOneoff.
	importPollOneoff = `(import "wasi_snapshot_preview1" "poll_oneoff"
    (func $wasi.poll_oneoff (param $in i32) (param $out i32) (param $nsubscriptions i32) (param $result.nevents i32) (result (;errno;) i32)))`

	// functionProcExit terminates the execution of the module with an exit code.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
	functionProcExit = "proc_exit"

	// importProcExit is the WebAssembly 1.0 (20191205) Text format import of functionProcExit.
	//
	// See importProcExit
	// See snapshotPreview1.ProcExit
	// See functionProcExit
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#proc_exit
	importProcExit = `(import "wasi_snapshot_preview1" "proc_exit"
    (func $wasi.proc_exit (param $rval i32)))`

	// functionProcRaise sends a signal to the process of the calling thread.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-proc_raisesig-signal---errno
	functionProcRaise = "proc_raise"

	// importProcRaise is the WebAssembly 1.0 (20191205) Text format import of functionProcRaise.
	importProcRaise = `(import "wasi_snapshot_preview1" "proc_raise"
    (func $wasi.proc_raise (param $sig i32) (result (;errno;) i32)))`

	// functionSchedYield temporarily yields execution of the calling thread.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sched_yield---errno
	functionSchedYield = "sched_yield"

	// importSchedYield is the WebAssembly 1.0 (20191205) Text format import of functionSchedYield.
	importSchedYield = `(import "wasi_snapshot_preview1" "sched_yield"
    (func $wasi.sched_yield (result (;errno;) i32)))`

	// functionRandomGet writes random data in buffer.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-buf_len-size---errno
	functionRandomGet = "random_get"

	// importRandomGet is the WebAssembly 1.0 (20191205) Text format import of functionRandomGet.
	importRandomGet = `(import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))`

	// functionSockRecv receives a message from a socket.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_recvfd-fd-ri_data-iovec_array-ri_flags-riflags---errno-size-roflags
	functionSockRecv = "sock_recv"

	// importSockRecv is the WebAssembly 1.0 (20191205) Text format import of functionSockRecv.
	importSockRecv = `(import "wasi_snapshot_preview1" "sock_recv"
    (func $wasi.sock_recv (param $fd i32) (param $ri_data i32) (param $ri_data_count i32) (param $ri_flags i32) (param $result.ro_datalen i32) (param $result.ro_flags i32) (result (;errno;) i32)))`

	// functionSockSend sends a message on a socket.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_sendfd-fd-si_data-ciovec_array-si_flags-siflags---errno-size
	functionSockSend = "sock_send"

	// importSockSend is the WebAssembly 1.0 (20191205) Text format import of functionSockSend.
	importSockSend = `(import "wasi_snapshot_preview1" "sock_send"
    (func $wasi.sock_send (param $fd i32) (param $si_data i32) (param $si_data_count i32) (param $si_flags i32) (param $result.so_datalen i32) (result (;errno;) i32)))`

	// functionSockShutdown shuts down socket send and receive channels.
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_shutdownfd-fd-how-sdflags---errno
	functionSockShutdown = "sock_shutdown"

	// importSockShutdown is the WebAssembly 1.0 (20191205) Text format import of functionSockShutdown.
	importSockShutdown = `(import "wasi_snapshot_preview1" "sock_shutdown"
    (func $wasi.sock_shutdown (param $fd i32) (param $how i32) (result (;errno;) i32)))`
)

// snapshotPreview1 includes all host functions to export for WASI version "wasi_snapshot_preview1".
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
type snapshotPreview1 interface {
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
	// Note: importArgsGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See ArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsGet(m api.Module, argv, argvBuf uint32) Errno

	// ArgsSizesGet is the WASI function named functionArgsSizesGet that reads command-line argument data (WithArgs)
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
	// Note: importArgsSizesGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See ArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#args_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	ArgsSizesGet(m api.Module, resultArgc, resultArgvBufSize uint32) Errno

	// EnvironGet is the WASI function named functionEnvironGet that reads environment variables. (WithEnviron)
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
	// Note: importEnvironGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See EnvironSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	EnvironGet(m api.Module, environ, environBuf uint32) Errno

	// EnvironSizesGet is the WASI function named functionEnvironSizesGet that reads environment variable
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
	// Note: importEnvironGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See EnvironGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#environ_sizes_get
	// See https://en.wikipedia.org/wiki/Null-terminated_string
	EnvironSizesGet(m api.Module, resultEnvironc, resultEnvironBufSize uint32) Errno

	// ClockResGet is the WASI function named functionClockResGet and is stubbed for GrainLang per #271
	ClockResGet(m api.Module, id uint32, resultResolution uint32) Errno

	// ClockTimeGet is the WASI function named functionClockTimeGet that returns the time value of a clock (time.Now).
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
	// Note: importClockTimeGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `clock_gettime` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	// See https://linux.die.net/man/3/clock_gettime
	ClockTimeGet(m api.Module, id uint32, precision uint64, resultTimestamp uint32) Errno

	// FdAdvise is the WASI function named functionFdAdvise and is stubbed for GrainLang per #271
	FdAdvise(m api.Module, fd uint32, offset, len uint64, resultAdvice uint32) Errno

	// FdAllocate is the WASI function named functionFdAllocate and is stubbed for GrainLang per #271
	FdAllocate(m api.Module, fd uint32, offset, len uint64) Errno

	// FdClose is the WASI function to close a file descriptor. This returns ErrnoBadf if the fd is invalid.
	//
	// * fd - the file descriptor to close
	//
	// Note: importFdClose shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `close` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_close
	// See https://linux.die.net/man/3/close
	FdClose(m api.Module, fd uint32) Errno

	// FdDatasync is the WASI function named functionFdDatasync and is stubbed for GrainLang per #271
	FdDatasync(m api.Module, fd uint32) Errno

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
	// Note: importFdFdstatGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: FdFdstatGet returns similar flags to `fsync(fd, F_GETFL)` in POSIX, as well as additional fields.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fdstat
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_fdstat_get
	// See https://linux.die.net/man/3/fsync
	FdFdstatGet(m api.Module, fd, resultFdstat uint32) Errno

	// FdFdstatSetFlags is the WASI function named functionFdFdstatSetFlags and is stubbed for GrainLang per #271
	FdFdstatSetFlags(m api.Module, fd uint32, flags uint32) Errno

	// FdFdstatSetRights is the WASI function named functionFdFdstatSetRights and is stubbed for GrainLang per #271
	FdFdstatSetRights(m api.Module, fd uint32, fsRightsBase, fsRightsInheriting uint64) Errno

	// FdFilestatGet is the WASI function named functionFdFilestatGet
	FdFilestatGet(m api.Module, fd uint32, resultBuf uint32) Errno

	// FdFilestatSetSize is the WASI function named functionFdFilestatSetSize
	FdFilestatSetSize(m api.Module, fd uint32, size uint64) Errno

	// FdFilestatSetTimes is the WASI function named functionFdFilestatSetTimes
	FdFilestatSetTimes(m api.Module, fd uint32, atim, mtim uint64, fstFlags uint32) Errno

	// FdPread is the WASI function named functionFdPread
	FdPread(m api.Module, fd, iovs, iovsCount uint32, offset uint64, resultNread uint32) Errno

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
	// Note: importFdPrestatGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See FdPrestatDirName
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#prestat
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_prestat_get
	FdPrestatGet(m api.Module, fd uint32, resultPrestat uint32) Errno

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
	// Note: importFdPrestatDirName shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See FdPrestatGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_prestat_dir_name
	FdPrestatDirName(m api.Module, fd, path, pathLen uint32) Errno
	// TODO: FdPrestatDirName may have to return ErrnoNotdir if the type of the prestat data of `fd` is not a PrestatDir.

	// FdPwrite is the WASI function named functionFdPwrite
	FdPwrite(m api.Module, fd, iovs, iovsCount uint32, offset uint64, resultNwritten uint32) Errno

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
	// Note: importFdRead shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `readv` in POSIX.
	// See FdWrite
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_read
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#iovec
	// See https://linux.die.net/man/3/readv
	FdRead(m api.Module, fd, iovs, iovsCount, resultSize uint32) Errno

	// FdReaddir is the WASI function named functionFdReaddir
	FdReaddir(m api.Module, fd, buf, bufLen uint32, cookie uint64, resultBufused uint32) Errno

	// FdRenumber is the WASI function named functionFdRenumber
	FdRenumber(m api.Module, fd, to uint32) Errno

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
	// Note: importFdSeek shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `lseek` in POSIX.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_seek
	// See https://linux.die.net/man/3/lseek
	FdSeek(m api.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) Errno

	// FdSync is the WASI function named functionFdSync
	FdSync(m api.Module, fd uint32) Errno

	// FdTell is the WASI function named functionFdTell
	FdTell(m api.Module, fd, resultOffset uint32) Errno

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
	// Note: importFdWrite shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `writev` in POSIX.
	// See FdRead
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#ciovec
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_write
	// See https://linux.die.net/man/3/writev
	FdWrite(m api.Module, fd, iovs, iovsCount, resultSize uint32) Errno

	// PathCreateDirectory is the WASI function named functionPathCreateDirectory
	PathCreateDirectory(m api.Module, fd, path, pathLen uint32) Errno

	// PathFilestatGet is the WASI function named functionPathFilestatGet
	PathFilestatGet(m api.Module, fd, flags, path, pathLen, resultBuf uint32) Errno

	// PathFilestatSetTimes is the WASI function named functionPathFilestatSetTimes
	PathFilestatSetTimes(m api.Module, fd, flags, path, pathLen uint32, atim, mtime uint64, fstFlags uint32) Errno

	// PathLink is the WASI function named functionPathLink
	PathLink(m api.Module, oldFd, oldFlags, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) Errno

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
	// Note: importPathOpen shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// Note: This is similar to `openat` in POSIX.
	// Note: The returned file descriptor is not guaranteed to be the lowest-numbered file
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#path_open
	// See https://linux.die.net/man/3/openat
	PathOpen(m api.Module, fd, dirflags, pathPtr, pathLen, oflags uint32, fsRightsBase,
		fsRightsInheriting uint64, fdflags, resultOpenedFd uint32) Errno

	// PathReadlink is the WASI function named functionPathReadlink
	PathReadlink(m api.Module, fd, path, pathLen, buf, bufLen, resultBufused uint32) Errno

	// PathRemoveDirectory is the WASI function named functionPathRemoveDirectory
	PathRemoveDirectory(m api.Module, fd, path, pathLen uint32) Errno

	// PathRename is the WASI function named functionPathRename
	PathRename(m api.Module, fd, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) Errno

	// PathSymlink is the WASI function named functionPathSymlink
	PathSymlink(m api.Module, oldPath, oldPathLen, fd, newPath, newPathLen uint32) Errno

	// PathUnlinkFile is the WASI function named functionPathUnlinkFile
	PathUnlinkFile(m api.Module, fd, path, pathLen uint32) Errno

	// PollOneoff is the WASI function named functionPollOneoff
	PollOneoff(m api.Module, in, out, nsubscriptions, resultNevents uint32) Errno

	// ProcExit is the WASI function that terminates the execution of the module with an exit code.
	// An exit code of 0 indicates successful termination. The meanings of other values are not defined by WASI.
	//
	// * rval - The exit code.
	//
	// In wazero, this calls api.Module CloseWithExitCode.
	//
	// Note: importProcExit shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
	ProcExit(m api.Module, rval uint32)

	// ProcRaise is the WASI function named functionProcRaise
	ProcRaise(m api.Module, sig uint32) Errno

	// SchedYield is the WASI function named functionSchedYield
	SchedYield(m api.Module) Errno

	// RandomGet is the WASI function named functionRandomGet that write random data in buffer (rand.Read()).
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
	// Note: importRandomGet shows this signature in the WebAssembly 1.0 (20191205) Text Format.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-bufLen-size---errno
	RandomGet(m api.Module, buf, bufLen uint32) Errno

	// SockRecv is the WASI function named functionSockRecv
	SockRecv(m api.Module, fd, riData, riDataCount, riFlags, resultRoDataLen, resultRoFlags uint32) Errno

	// SockSend is the WASI function named functionSockSend
	SockSend(m api.Module, fd, siData, siDataCount, siFlags, resultSoDataLen uint32) Errno

	// SockShutdown is the WASI function named functionSockShutdown
	SockShutdown(m api.Module, fd, how uint32) Errno
}

var _ snapshotPreview1 = &wasiAPI{}

type wasiAPI struct {
	// timeNowUnixNano is mutable for testing
	timeNowUnixNano func() uint64
	randSource      func([]byte) error
}

// snapshotPreview1Functions returns all go functions that implement snapshotPreview1.
// These should be exported in the module named "wasi_snapshot_preview1".
// See wasm.NewHostModule
func snapshotPreview1Functions() (a *wasiAPI, nameToGoFunc map[string]interface{}) {
	a = newAPI()
	// Note: these are ordered per spec for consistency even if the resulting map can't guarantee that.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#functions
	nameToGoFunc = map[string]interface{}{
		functionArgsGet:              a.ArgsGet,
		functionArgsSizesGet:         a.ArgsSizesGet,
		functionEnvironGet:           a.EnvironGet,
		functionEnvironSizesGet:      a.EnvironSizesGet,
		functionClockResGet:          a.ClockResGet,
		functionClockTimeGet:         a.ClockTimeGet,
		functionFdAdvise:             a.FdAdvise,
		functionFdAllocate:           a.FdAllocate,
		functionFdClose:              a.FdClose,
		functionFdDatasync:           a.FdDatasync,
		functionFdFdstatGet:          a.FdFdstatGet,
		functionFdFdstatSetFlags:     a.FdFdstatSetFlags,
		functionFdFdstatSetRights:    a.FdFdstatSetRights,
		functionFdFilestatGet:        a.FdFilestatGet,
		functionFdFilestatSetSize:    a.FdFilestatSetSize,
		functionFdFilestatSetTimes:   a.FdFilestatSetTimes,
		functionFdPread:              a.FdPread,
		functionFdPrestatGet:         a.FdPrestatGet,
		functionFdPrestatDirName:     a.FdPrestatDirName,
		functionFdPwrite:             a.FdPwrite,
		functionFdRead:               a.FdRead,
		functionFdReaddir:            a.FdReaddir,
		functionFdRenumber:           a.FdRenumber,
		functionFdSeek:               a.FdSeek,
		functionFdSync:               a.FdSync,
		functionFdTell:               a.FdTell,
		functionFdWrite:              a.FdWrite,
		functionPathCreateDirectory:  a.PathCreateDirectory,
		functionPathFilestatGet:      a.PathFilestatGet,
		functionPathFilestatSetTimes: a.PathFilestatSetTimes,
		functionPathLink:             a.PathLink,
		functionPathOpen:             a.PathOpen,
		functionPathReadlink:         a.PathReadlink,
		functionPathRemoveDirectory:  a.PathRemoveDirectory,
		functionPathRename:           a.PathRename,
		functionPathSymlink:          a.PathSymlink,
		functionPathUnlinkFile:       a.PathUnlinkFile,
		functionPollOneoff:           a.PollOneoff,
		functionProcExit:             a.ProcExit,
		functionProcRaise:            a.ProcRaise,
		functionSchedYield:           a.SchedYield,
		functionRandomGet:            a.RandomGet,
		functionSockRecv:             a.SockRecv,
		functionSockSend:             a.SockSend,
		functionSockShutdown:         a.SockShutdown,
	}
	return
}

// ArgsGet implements snapshotPreview1.ArgsGet
func (a *wasiAPI) ArgsGet(m api.Module, argv, argvBuf uint32) Errno {
	sys := sysCtx(m)
	return writeOffsetsAndNullTerminatedValues(m.Memory(), sys.Args(), argv, argvBuf)
}

// ArgsSizesGet implements snapshotPreview1.ArgsSizesGet
func (a *wasiAPI) ArgsSizesGet(m api.Module, resultArgc, resultArgvBufSize uint32) Errno {
	sys := sysCtx(m)
	mem := m.Memory()

	if !mem.WriteUint32Le(resultArgc, uint32(len(sys.Args()))) {
		return ErrnoFault
	}
	if !mem.WriteUint32Le(resultArgvBufSize, sys.ArgsSize()) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// EnvironGet implements snapshotPreview1.EnvironGet
func (a *wasiAPI) EnvironGet(m api.Module, environ uint32, environBuf uint32) Errno {
	sys := sysCtx(m)
	return writeOffsetsAndNullTerminatedValues(m.Memory(), sys.Environ(), environ, environBuf)
}

// EnvironSizesGet implements snapshotPreview1.EnvironSizesGet
func (a *wasiAPI) EnvironSizesGet(m api.Module, resultEnvironc uint32, resultEnvironBufSize uint32) Errno {
	sys := sysCtx(m)
	mem := m.Memory()

	if !mem.WriteUint32Le(resultEnvironc, uint32(len(sys.Environ()))) {
		return ErrnoFault
	}
	if !mem.WriteUint32Le(resultEnvironBufSize, sys.EnvironSize()) {
		return ErrnoFault
	}

	return ErrnoSuccess
}

// ClockResGet implements snapshotPreview1.ClockResGet
func (a *wasiAPI) ClockResGet(m api.Module, id uint32, resultResolution uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// ClockTimeGet implements snapshotPreview1.ClockTimeGet
func (a *wasiAPI) ClockTimeGet(m api.Module, id uint32, precision uint64, resultTimestamp uint32) Errno {
	// TODO: id and precision are currently ignored.
	if !m.Memory().WriteUint64Le(resultTimestamp, a.timeNowUnixNano()) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// FdAdvise implements snapshotPreview1.FdAdvise
func (a *wasiAPI) FdAdvise(m api.Module, fd uint32, offset, len uint64, resultAdvice uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdAllocate implements snapshotPreview1.FdAllocate
func (a *wasiAPI) FdAllocate(m api.Module, fd uint32, offset, len uint64) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdClose implements snapshotPreview1.FdClose
func (a *wasiAPI) FdClose(m api.Module, fd uint32) Errno {
	sys := sysCtx(m)

	if ok, err := sys.CloseFile(fd); err != nil {
		return ErrnoIo
	} else if !ok {
		return ErrnoBadf
	}

	return ErrnoSuccess
}

// FdDatasync implements snapshotPreview1.FdDatasync
func (a *wasiAPI) FdDatasync(m api.Module, fd uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdFdstatGet implements snapshotPreview1.FdFdstatGet
func (a *wasiAPI) FdFdstatGet(m api.Module, fd uint32, resultStat uint32) Errno {
	sys := sysCtx(m)

	if _, ok := sys.OpenedFile(fd); !ok {
		return ErrnoBadf
	}
	return ErrnoSuccess
}

// FdPrestatGet implements snapshotPreview1.FdPrestatGet
func (a *wasiAPI) FdPrestatGet(m api.Module, fd uint32, resultPrestat uint32) Errno {
	sys := sysCtx(m)

	entry, ok := sys.OpenedFile(fd)
	if !ok {
		return ErrnoBadf
	}

	// Zero-value 8-bit tag, and 3-byte zero-value paddings, which is uint32le(0) in short.
	if !m.Memory().WriteUint32Le(resultPrestat, uint32(0)) {
		return ErrnoFault
	}
	// Write the length of the directory name at offset 4.
	if !m.Memory().WriteUint32Le(resultPrestat+4, uint32(len(entry.Path))) {
		return ErrnoFault
	}

	return ErrnoSuccess
}

// FdFdstatSetFlags implements snapshotPreview1.FdFdstatSetFlags
func (a *wasiAPI) FdFdstatSetFlags(m api.Module, fd uint32, flags uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdFdstatSetRights implements snapshotPreview1.FdFdstatSetRights
// Note: This will never be implemented per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
func (a *wasiAPI) FdFdstatSetRights(m api.Module, fd uint32, fsRightsBase, fsRightsInheriting uint64) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatGet implements snapshotPreview1.FdFilestatGet
func (a *wasiAPI) FdFilestatGet(m api.Module, fd uint32, resultBuf uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatSetSize implements snapshotPreview1.FdFilestatSetSize
func (a *wasiAPI) FdFilestatSetSize(m api.Module, fd uint32, size uint64) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdFilestatSetTimes implements snapshotPreview1.FdFilestatSetTimes
func (a *wasiAPI) FdFilestatSetTimes(m api.Module, fd uint32, atim, mtim uint64, fstFlags uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdPread implements snapshotPreview1.FdPread
func (a *wasiAPI) FdPread(m api.Module, fd, iovs, iovsCount uint32, offset uint64, resultNread uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdPrestatDirName implements snapshotPreview1.FdPrestatDirName
func (a *wasiAPI) FdPrestatDirName(m api.Module, fd uint32, pathPtr uint32, pathLen uint32) Errno {
	sys := sysCtx(m)

	f, ok := sys.OpenedFile(fd)
	if !ok {
		return ErrnoBadf
	}

	// Some runtimes may have another semantics. See internal/wasi/RATIONALE.md
	if uint32(len(f.Path)) < pathLen {
		return ErrnoNametoolong
	}

	// TODO: FdPrestatDirName may have to return ErrnoNotdir if the type of the prestat data of `fd` is not a PrestatDir.
	if !m.Memory().Write(pathPtr, []byte(f.Path)[:pathLen]) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// FdPwrite implements snapshotPreview1.FdPwrite
func (a *wasiAPI) FdPwrite(m api.Module, fd, iovs, iovsCount uint32, offset uint64, resultNwritten uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdRead implements snapshotPreview1.FdRead
func (a *wasiAPI) FdRead(m api.Module, fd, iovs, iovsCount, resultSize uint32) Errno {
	sys := sysCtx(m)

	var reader io.Reader

	if fd == fdStdin {
		reader = sys.Stdin()
	} else if f, ok := sys.OpenedFile(fd); !ok || f.File == nil {
		return ErrnoBadf
	} else {
		reader = f.File
	}

	var nread uint32
	for i := uint32(0); i < iovsCount; i++ {
		iovPtr := iovs + i*8
		offset, ok := m.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return ErrnoFault
		}
		l, ok := m.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return ErrnoFault
		}
		b, ok := m.Memory().Read(offset, l)
		if !ok {
			return ErrnoFault
		}
		n, err := reader.Read(b)
		nread += uint32(n)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return ErrnoIo
		}
	}
	if !m.Memory().WriteUint32Le(resultSize, nread) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// FdReaddir implements snapshotPreview1.FdReaddir
func (a *wasiAPI) FdReaddir(m api.Module, fd, buf, bufLen uint32, cookie uint64, resultBufused uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdRenumber implements snapshotPreview1.FdRenumber
func (a *wasiAPI) FdRenumber(m api.Module, fd, to uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdSeek implements snapshotPreview1.FdSeek
func (a *wasiAPI) FdSeek(m api.Module, fd uint32, offset uint64, whence uint32, resultNewoffset uint32) Errno {
	sys := sysCtx(m)

	var seeker io.Seeker
	// Check to see if the file descriptor is available
	if f, ok := sys.OpenedFile(fd); !ok || f.File == nil {
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

	if !m.Memory().WriteUint32Le(resultNewoffset, uint32(newOffset)) {
		return ErrnoFault
	}

	return ErrnoSuccess
}

// FdSync implements snapshotPreview1.FdSync
func (a *wasiAPI) FdSync(m api.Module, fd uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdTell implements snapshotPreview1.FdTell
func (a *wasiAPI) FdTell(m api.Module, fd, resultOffset uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// FdWrite implements snapshotPreview1.FdWrite
func (a *wasiAPI) FdWrite(m api.Module, fd, iovs, iovsCount, resultSize uint32) Errno {
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
			return ErrnoBadf
			// fs.FS doesn't declare io.Writer, but implementations such as os.File implement it.
		} else if writer, ok = f.File.(io.Writer); !ok {
			return ErrnoBadf
		}
	}

	var nwritten uint32
	for i := uint32(0); i < iovsCount; i++ {
		iovPtr := iovs + i*8
		offset, ok := m.Memory().ReadUint32Le(iovPtr)
		if !ok {
			return ErrnoFault
		}
		l, ok := m.Memory().ReadUint32Le(iovPtr + 4)
		if !ok {
			return ErrnoFault
		}
		b, ok := m.Memory().Read(offset, l)
		if !ok {
			return ErrnoFault
		}
		n, err := writer.Write(b)
		if err != nil {
			return ErrnoIo
		}
		nwritten += uint32(n)
	}
	if !m.Memory().WriteUint32Le(resultSize, nwritten) {
		return ErrnoFault
	}
	return ErrnoSuccess
}

// PathCreateDirectory implements snapshotPreview1.PathCreateDirectory
func (a *wasiAPI) PathCreateDirectory(m api.Module, fd, path, pathLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathFilestatGet implements snapshotPreview1.PathFilestatGet
func (a *wasiAPI) PathFilestatGet(m api.Module, fd, flags, path, pathLen, resultBuf uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathFilestatSetTimes implements snapshotPreview1.PathFilestatSetTimes
func (a *wasiAPI) PathFilestatSetTimes(m api.Module, fd, flags, path, pathLen uint32, atim, mtime uint64, fstFlags uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathLink implements snapshotPreview1.PathLink
func (a *wasiAPI) PathLink(m api.Module, oldFd, oldFlags, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathOpen implements snapshotPreview1.PathOpen
// Note: Rights will never be implemented per https://github.com/WebAssembly/WASI/issues/469#issuecomment-1045251844
func (a *wasiAPI) PathOpen(m api.Module, fd, dirflags, pathPtr, pathLen, oflags uint32, fsRightsBase,
	fsRightsInheriting uint64, fdflags, resultOpenedFd uint32) (errno Errno) {
	sys := sysCtx(m)

	dir, ok := sys.OpenedFile(fd)
	if !ok || dir.FS == nil {
		return ErrnoBadf
	}

	b, ok := m.Memory().Read(pathPtr, pathLen)
	if !ok {
		return ErrnoFault
	}

	// TODO: Consider dirflags and oflags. Also, allow non-read-only open based on config about the mount.
	// Ex. allow os.O_RDONLY, os.O_WRONLY, or os.O_RDWR either by config flag or pattern on filename
	// See #390
	entry, errno := openFileEntry(dir.FS, path.Join(dir.Path, string(b)))
	if errno != ErrnoSuccess {
		return errno
	}

	if newFD, ok := sys.OpenFile(entry); !ok {
		_ = entry.File.Close()
		return ErrnoIo
	} else if !m.Memory().WriteUint32Le(resultOpenedFd, newFD) {
		_ = entry.File.Close()
		return ErrnoFault
	}
	return ErrnoSuccess
}

// PathReadlink implements snapshotPreview1.PathReadlink
func (a *wasiAPI) PathReadlink(m api.Module, fd, path, pathLen, buf, bufLen, resultBufused uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathRemoveDirectory implements snapshotPreview1.PathRemoveDirectory
func (a *wasiAPI) PathRemoveDirectory(m api.Module, fd, path, pathLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathRename implements snapshotPreview1.PathRename
func (a *wasiAPI) PathRename(m api.Module, fd, oldPath, oldPathLen, newFd, newPath, newPathLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathSymlink implements snapshotPreview1.PathSymlink
func (a *wasiAPI) PathSymlink(m api.Module, oldPath, oldPathLen, fd, newPath, newPathLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PathUnlinkFile implements snapshotPreview1.PathUnlinkFile
func (a *wasiAPI) PathUnlinkFile(m api.Module, fd, path, pathLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// PollOneoff implements snapshotPreview1.PollOneoff
func (a *wasiAPI) PollOneoff(m api.Module, in, out, nsubscriptions, resultNevents uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// ProcExit implements snapshotPreview1.ProcExit
func (a *wasiAPI) ProcExit(m api.Module, exitCode uint32) {
	_ = m.CloseWithExitCode(exitCode)
}

// ProcRaise implements snapshotPreview1.ProcRaise
func (a *wasiAPI) ProcRaise(m api.Module, sig uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// SchedYield implements snapshotPreview1.SchedYield
func (a *wasiAPI) SchedYield(m api.Module) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// RandomGet implements snapshotPreview1.RandomGet
func (a *wasiAPI) RandomGet(m api.Module, buf uint32, bufLen uint32) (errno Errno) {
	randomBytes := make([]byte, bufLen)
	err := a.randSource(randomBytes)
	if err != nil {
		// TODO: handle different errors that syscal to entropy source can return
		return ErrnoIo
	}

	if !m.Memory().Write(buf, randomBytes) {
		return ErrnoFault
	}

	return ErrnoSuccess
}

// SockRecv implements snapshotPreview1.SockRecv
func (a *wasiAPI) SockRecv(m api.Module, fd, riData, riDataCount, riFlags, resultRoDataLen, resultRoFlags uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// SockSend implements snapshotPreview1.SockSend
func (a *wasiAPI) SockSend(m api.Module, fd, siData, siDataCount, siFlags, resultSoDataLen uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

// SockShutdown implements snapshotPreview1.SockShutdown
func (a *wasiAPI) SockShutdown(m api.Module, fd, how uint32) Errno {
	return ErrnoNosys // stubbed for GrainLang per #271
}

const (
	fdStdin  = 0
	fdStdout = 1
	fdStderr = 2
)

// newAPI is exported for benchmarks
func newAPI() *wasiAPI {
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

func sysCtx(m api.Module) *wasm.SysContext {
	if internal, ok := m.(*wasm.ModuleContext); !ok {
		panic(fmt.Errorf("unsupported wasm.Module implementation: %v", m))
	} else {
		return internal.Sys()
	}
}

func openFileEntry(rootFS fs.FS, pathName string) (*wasm.FileEntry, Errno) {
	f, err := rootFS.Open(pathName)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, ErrnoNoent
		case errors.Is(err, fs.ErrExist):
			return nil, ErrnoExist
		default:
			return nil, ErrnoIo
		}
	}

	// TODO: verify if oflags is a directory and fail with wasi.ErrnoNotdir if not
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-oflags-flagsu16

	return &wasm.FileEntry{Path: pathName, FS: rootFS, File: f}, ErrnoSuccess
}

func writeOffsetsAndNullTerminatedValues(mem api.Memory, values []string, offsets, bytes uint32) Errno {
	for _, value := range values {
		// Write current offset and advance it.
		if !mem.WriteUint32Le(offsets, bytes) {
			return ErrnoFault
		}
		offsets += 4 // size of uint32

		// Write the next value to memory with a NUL terminator
		if !mem.Write(bytes, []byte(value)) {
			return ErrnoFault
		}
		bytes += uint32(len(value))
		if !mem.WriteByte(bytes, 0) {
			return ErrnoFault
		}
		bytes++
	}

	return ErrnoSuccess
}
