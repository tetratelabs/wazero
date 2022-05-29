(module
  (func ;; re-export fd_prestat_dir_name, imported from WASI
    (export "clock_time_get")
    (import "wasi_snapshot_preview1" "clock_time_get")
    (param $id i32) (param $precision i64) (param $result.timestamp i32) (result (;errno;) i32)
  )

  (memory (export "memory") 1 1) ;; memory is required for WASI
)
