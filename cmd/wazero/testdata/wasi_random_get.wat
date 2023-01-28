(module $listener
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))

  (memory 1 1) ;; Memory is needed for WASI

  ;; _start is a special function defined by a WASI Command that runs like a main function would.
  ;;
  ;; See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
  (func $main (export "_start")
      ;; generate 1000 bytes of random data starting at address 0
      (call $wasi.random_get
          (i32.const 0)
          (i32.const 1000)
      )
      drop
  )
)
