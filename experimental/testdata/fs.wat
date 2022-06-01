(module
  (func ;; re-export fd_prestat_dir_name, imported from WASI
    (export "fd_prestat_dir_name")
    (import "wasi_snapshot_preview1" "fd_prestat_dir_name")
    (param $fd i32) (param $path i32) (param $path_len i32) (result (;errno;) i32)
  )

  (memory (export "memory") 1 1) ;; memory is required for WASI
)
