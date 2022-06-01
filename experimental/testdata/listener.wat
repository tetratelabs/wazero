(module $listener
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))
  (func i32.const 0 i32.const 4 call 0 drop) ;; write 4 bytes of random data
  (memory 1 1)
  (start 1) ;; call the second function
)
