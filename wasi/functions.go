package wasi

const (
	// ModuleSnapshotPreview1 is the module name WASI functions are exported into
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
	ModuleSnapshotPreview1 = "wasi_snapshot_preview1"

	// FunctionArgsGet reads command-line argument data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_getargv-pointerpointeru8-argv_buf-pointeru8---errno
	FunctionArgsGet = "args_get"

	// FunctionArgsSizesGet returns command-line argument data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
	FunctionArgsSizesGet = "args_sizes_get"

	// FunctionEnvironGet reads environment variable data.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_getenviron-pointerpointeru8-environ_buf-pointeru8---errno
	FunctionEnvironGet = "environ_get"

	// FunctionEnvironSizesGet returns environment variable data sizes.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-environ_sizes_get---errno-size-size
	FunctionEnvironSizesGet = "environ_sizes_get"

	// FunctionClockResGet returns the resolution of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_res_getid-clockid---errno-timestamp
	FunctionClockResGet = "clock_res_get"

	// FunctionClockTimeGet returns the time value of a clock.
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-clock_time_getid-clockid-precision-timestamp---errno-timestamp
	FunctionClockTimeGet = "clock_time_get"

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
	// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-random_getbuf-pointeru8-buf_len-size---errno
	FunctionRandomGet = "random_get"

	FunctionSockRecv     = "sock_recv"
	FunctionSockSend     = "sock_send"
	FunctionSockShutdown = "sock_shutdown"
)
