;; This file is a listener example, compiled to work with WebAssembly 1.0:
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory \
;;      --disable-reference-types \
;;      --debug-names \
;;      listener.wat
(module $listener
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))

  (table 8 funcref) ;; Define a function table with a single element at index 3.
  (elem (i32.const 3) $wasi.random_get)

  (memory 1 1) ;; Memory is needed for WASI

  (func $rand (export "rand") (param $len i32)
    i32.const 4 local.get 0 ;; buf, buf_len
    call $wasi.random_get
    drop ;; errno

    i32.const 8 local.get 0 ;; buf, buf_len
    i32.const 3 call_indirect (type 0) ;; element 3, func type 0
    drop ;; errno
  )
)
