(module $wasi_fs_exports

  (func (export "path_open")
    (import "wasi_snapshot_preview1" "path_open")
    (param $fd i32) (param $dirflags i32) (param $path i32) (param $path_len i32) (param $oflags i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (param $fdflags i32) (param $result.opened_fd i32) (result (;errno;) i32))

  (func (export "fd_close")
    (import "wasi_snapshot_preview1" "fd_close")
    (param $fd i32) (result (;errno;) i32))

  (func (export "fd_read")
    (import "wasi_snapshot_preview1" "fd_read")
    (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32))

  (func (export "fd_seek")
    (import "wasi_snapshot_preview1" "fd_seek")
    (param $fd i32) (param $offset i64) (param $whence i32) (param $result.newoffset i32) (result (;errno;) i32))

  (memory (export "memory") 1 1) ;; memory is required for WASI
)
