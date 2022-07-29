(module
  (type $tp (func (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)))
  (import "wasi_snapshot_preview1" "fd_prestat_dir_name" (func $fd_prestat_dir_name (type $tp)))
  (func (export "fd_prestat_dir_name") (type $tp)
    (call $fd_prestat_dir_name (local.get 0) (local.get 1) (local.get 2))
  )
  (memory (export "memory") 1 1) ;; memory is required for WASI
)
