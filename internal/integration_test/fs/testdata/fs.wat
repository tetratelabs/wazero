(module $wasi_fs_exports
  (type $type_path_open (func (param $fd i32) (param $dirflags i32) (param $path i32) (param $path_len i32) (param $oflags i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (param $fdflags i32) (param $result.opened_fd i32) (result (;errno;) i32)))
  (type $type_fd_close (func (param $fd i32) (result (;errno;) i32)))
  (type $type_fd_read (func (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))
  (type $type_fd_seek (func (param $fd i32) (param $offset i64) (param $whence i32) (param $result.newoffset i32) (result (;errno;) i32)))

  (import "wasi_snapshot_preview1" "path_open" (func $path_open (type $type_path_open)))
  (import "wasi_snapshot_preview1" "fd_close" (func $fd_close (type $type_fd_close)))
  (import "wasi_snapshot_preview1" "fd_read" (func $fd_read (type $type_fd_read)))
  (import "wasi_snapshot_preview1" "fd_seek" (func $fd_seek (type $type_fd_seek)))

  (func (export "path_open") (type $type_path_open)
    local.get 0 local.get 1 local.get 2 local.get 3 local.get 4 local.get 5 local.get 6 local.get 7 local.get 8
    call $path_open
  )
  (func (export "fd_close") (type $type_fd_close)
    local.get 0
    call $fd_close
  )
  (func (export "fd_read") (type $type_fd_read)
    local.get 0 local.get 1 local.get 2 local.get 3
    call $fd_read
  )
  (func (export "fd_seek") (type $type_fd_seek)
    local.get 0 local.get 1 local.get 2 local.get 3
    call $fd_seek
  )

  (memory (export "memory") 1 1) ;; memory is required for WASI
)
