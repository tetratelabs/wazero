package wasi

const (
	// ModuleSnapshotPreview1 is the module name WASI functions are exported into
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
	ModuleSnapshotPreview1 = "wasi_snapshot_preview1"

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
	//
	// See ImportArgsGet
	// See FunctionArgsSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	FunctionArgsGet = "args_get"

	// ImportArgsGet is the WebAssembly 1.0 (MVP) Text format import of FunctionArgsGet
	ImportArgsGet = `(import "wasi_snapshot_preview1" "args_get"
    (func $wasi.args_get (param $argv i32) (param $argv_buf i32) (result (;errno;) i32)))`

	// FunctionArgsSizesGet returns command-line argument data sizes.
	//
	// See ImportArgsSizesGet
	// See FunctionArgsGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	FunctionArgsSizesGet = "args_sizes_get"

	// ImportArgsSizesGet is the WebAssembly 1.0 (MVP) Text format import of FunctionArgsSizesGet
	ImportArgsSizesGet = `(import "wasi_snapshot_preview1" "args_sizes_get"
    (func $wasi.args_sizes_get (param $result.argc i32) (param $result.argv_buf_size i32) (result (;errno;) i32)))`

	// FunctionEnvironGet reads environment variable data.
	//
	// See ImportEnvironGet
	// See FunctionEnvironSizesGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	FunctionEnvironGet = "environ_get"

	// ImportEnvironGet is the WebAssembly 1.0 (MVP) Text format import of FunctionEnvironGet
	ImportEnvironGet = `(import "wasi_snapshot_preview1" "environ_get"
    (func $wasi.environ_get (param $environ i32) (param $environ_buf i32) (result (;errno;) i32)))`

	// FunctionEnvironSizesGet returns environment variable data sizes.
	//
	// See ImportEnvironSizesGet
	// See FunctionEnvironGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	FunctionEnvironSizesGet = "environ_sizes_get"

	// ImportEnvironSizesGet is the WebAssembly 1.0 (MVP) Text format import of FunctionEnvironSizesGet
	ImportEnvironSizesGet = `
(import "wasi_snapshot_preview1" "environ_sizes_get"
    (func $wasi.environ_sizes_get (param $result.environc i32) (param $result.environBufSize i32) (result (;errno;) i32)))`

	// FunctionClockResGet returns the resolution of a clock.
	//
	// See ImportClockResGet
	// See FunctionClockTimeGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	FunctionClockResGet = "clock_res_get"

	// ImportClockResGet is the WebAssembly 1.0 (MVP) Text format import of FunctionClockResGet
	ImportClockResGet = `
(import "wasi_snapshot_preview1" "clock_res_get"
    (func $wasi.clock_res_get (param $id i32) (param $result.resolution i32) (result (;errno;) i32)))`

	// FunctionClockTimeGet returns the time value of a clock.
	//
	// See ImportClockTimeGet
	// See FunctionClockResGet
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	FunctionClockTimeGet = "clock_time_get"

	// ImportClockTimeGet is the WebAssembly 1.0 (MVP) Text format import of FunctionClockTimeGet
	ImportClockTimeGet = `(import "wasi_snapshot_preview1" "clock_time_get"
    (func $wasi.clock_time_get (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)))`

	FunctionFdAdvise             = "fd_advise"
	FunctionFdAllocate           = "fd_allocate"
	FunctionFdClose              = "fd_close"
	FunctionFdDataSync           = "fd_datasync"
	FunctionFdFdstatGet          = "fd_fdstat_get"
	FunctionFdFdstatSetFlags     = "fd_fdstat_set_flags"
	FunctionFdFdstatSetRights    = "fd_fdstat_set_rights"
	FunctionFdFilestatGet        = "fd_filestat_get"
	FunctionFdFilestatSetSize    = "fd_filestat_set_size"
	FunctionFdFilestatSetTimes   = "fd_filestat_set_times"
	FunctionFdPread              = "fd_pread"
	FunctionFdPrestatGet         = "fd_prestat_get"
	FunctionFdPrestatDirName     = "fd_prestat_dir_name"
	FunctionFdPwrite             = "fd_pwrite"
	FunctionFdRead               = "fd_read"
	FunctionFdReaddir            = "fd_readdir"
	FunctionFdRenumber           = "fd_renumber"
	FunctionFdSeek               = "fd_seek"
	FunctionFdSync               = "fd_sync"
	FunctionFdTell               = "fd_tell"
	FunctionFdWrite              = "fd_write"
	FunctionPathCreateDirectory  = "path_create_directory"
	FunctionPathFilestatGet      = "path_filestat_get"
	FunctionPathFilestatSetTimes = "path_filestat_set_times"
	FunctionPathLink             = "path_link"
	FunctionPathOpen             = "path_open"
	FunctionPathReadlink         = "path_readlink"
	FunctionPathRemoveDirectory  = "path_remove_directory"
	FunctionPathRename           = "path_rename"
	FunctionPathSymlink          = "path_symlink"
	FunctionPathUnlinkFile       = "path_unlink_file"
	FunctionPollOneoff           = "poll_oneoff"
	FunctionProcExit             = "proc_exit"
	FunctionProcRaise            = "proc_raise"
	FunctionSchedYield           = "sched_yield"

	// FunctionRandomGet write random data in buffer
	//
	// See ImportRandomGet
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-buf_len-size---errno
	FunctionRandomGet = "random_get"

	// ImportRandomGet is the WebAssembly 1.0 (MVP) Text format import of FunctionRandomGet
	ImportRandomGet = `(import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))`

	FunctionSockRecv     = "sock_recv"
	FunctionSockSend     = "sock_send"
	FunctionSockShutdown = "sock_shutdown"
)
