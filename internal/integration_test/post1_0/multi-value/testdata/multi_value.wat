;; multiValue is a WebAssembly 1.0 (20191205) Text Format source, plus the "multi-value" feature.
;; * allows multiple return values from `func`, `block`, `loop` and `if`
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      --debug-names multi_value.wat
;;
;; See https://github.com/WebAssembly/spec/blob/main/proposals/multi-value/Overview.md
(module $multi-value
  (func $swap (param i32 i32) (result i32 i32)
    (local.get 1) (local.get 0)
  )
  (export "swap" (func $swap))

  (func $add64_u_with_carry (param $i i64) (param $j i64) (param $c i32) (result i64 i32)
    (local $k i64)
    (local.set $k
      (i64.add (i64.add (local.get $i) (local.get $j)) (i64.extend_i32_u (local.get $c)))
    )
    (return (local.get $k) (i64.lt_u (local.get $k) (local.get $i)))
  )
  (export "add64_u_with_carry" (func $add64_u_with_carry))

  (func $add64_u_saturated (param i64 i64) (result i64)
    (call $add64_u_with_carry (local.get 0) (local.get 1) (i32.const 0))
    (if (param i64) (result i64)
      (then (drop) (i64.const 0xffff_ffff_ffff_ffff))
    )
  )
  (export "add64_u_saturated" (func $add64_u_saturated))

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
