;; bulk-memory-operations is a WebAssembly 1.0 (20191205) Text Format source,
;; plus the "bulk-memory-operations" feature. The module here corresponds to
;; the example in Overview.md.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-multi-value \
;;      bulk_memory_operations.wat
;;
;; See https://github.com/WebAssembly/spec/blob/main/proposals/bulk-memory-operations/Overview.md
(module $bulk-memory-operations
  (import "a" "global" (global i32))  ;; global 0
  (memory 1)
  (data (i32.const 0) "hello")   ;; data segment 0, is active so always copied
  (data "goodbye")               ;; data segment 1, is passive

  (func $start
    (if (global.get 0)

      ;; copy data segment 1 into memory 0 (the 0 is implicit)
      (memory.init 1
        (i32.const 16)    ;; target offset
        (i32.const 0)     ;; source offset
        (i32.const 7))    ;; length

      ;; The memory used by this segment is no longer needed, so this segment can
      ;; be dropped.
      (data.drop 1))
  )
  (start $start)
)
