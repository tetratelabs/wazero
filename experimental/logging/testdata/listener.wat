(module $listener
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))

  (import "wasi_snapshot_preview1" "fd_write"
    (func $wasi.fd_write (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))

  (table 8 funcref) ;; Define a function table with a single element at index 3.
  (elem (i32.const 3) $wasi.random_get)

  (memory 1 1) ;; Memory is needed for WASI

  (func $wasi_rand (param $len i32)
    ;; call fd_write with an unsampled FD, inside a sampled function.
    i32.const 1 ;; $fd = stdout
    i32.const 0 i32.const 0 i32.const 0 ;; $iovs, $iovs_len $result.size = 0.
    call $wasi.fd_write
    drop ;; errno

    i32.const 4 local.get 0 ;; buf, buf_len
    call $wasi.random_get
    drop ;; errno

    i32.const 8 local.get 0 ;; buf, buf_len
    i32.const 3 call_indirect (type 0) ;; element 3, func type 0
    drop ;; errno
  )

  (func $rand (export "rand") (param $len i32)
    local.get 0 ;; buf_len
    call $wasi_rand
  )
)
