;; This includes a factorial function that uses the "multi-value" feature
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      --debug-names fac.wat
;;
;; See https://github.com/WebAssembly/spec/blob/main/proposals/multi-value/Overview.md
(module
  (func $pick0 (param i64) (result i64 i64)
    (local.get 0) (local.get 0)
  )

  (func $pick1 (param i64 i64) (result i64 i64 i64)
    (local.get 0) (local.get 1) (local.get 0)
  )

  ;; Note: This implementation loops forever if the input is zero.
  (func $fac (param i64) (result i64)
    (i64.const 1) (local.get 0)

    (loop $l (param i64 i64) (result i64)
      (call $pick1) (call $pick1) (i64.mul)
      (call $pick1) (i64.const 1) (i64.sub)
      (call $pick0) (i64.const 0) (i64.gt_u)
      (br_if $l)
      (drop) (return)
    )
  )
  (export "fac" (func $fac))
)
