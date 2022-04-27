;; This file includes changes to test/core/func.wast from the commit that added "multi-value" support.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      func.wat
;;
;; See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
(module $func.wast
;; preconditions
  (func $dummy)

;; changes
  (func (result))
  (func (result) (result))
  (func (result i32 f64 f32) (unreachable))
  (func (result i32) (result f64) (unreachable))
  (func (result i32 f32) (result i64) (result) (result i32 f64) (unreachable))

  (func $complex
    (param i32 f32) (param $x i64) (param) (param i32)
    (result) (result i32) (result) (result i64 i32)
    (local f32) (local $y i32) (local i64 i32) (local) (local f64 i32)
    (unreachable) (unreachable)
  )

  (func (export "value-i32-f64") (result i32 f64) (i32.const 77) (f64.const 7))
  (func (export "value-i32-i32-i32") (result i32 i32 i32)
    (i32.const 1) (i32.const 2) (i32.const 3)
  )

  (func (export "value-block-i32-i64") (result i32 i64)
    (block (result i32 i64) (call $dummy) (i32.const 1) (i64.const 2))
  )

  (func (export "return-i32-f64") (result i32 f64)
    (return (i32.const 78) (f64.const 78.78))
  )

  (func (export "return-i32-i32-i32") (result i32 i32 i32)
    (return (i32.const 1) (i32.const 2) (i32.const 3))
  )

  (func (export "return-block-i32-i64") (result i32 i64)
    (return (block (result i32 i64) (call $dummy) (i32.const 1) (i64.const 2)))
  )

  (func (export "break-i32-f64") (result i32 f64)
    (br 0 (i32.const 79) (f64.const 79.79))
  )

  (func (export "break-i32-i32-i32") (result i32 i32 i32)
    (br 0 (i32.const 1) (i32.const 2) (i32.const 3))
  )

  (func (export "break-block-i32-i64") (result i32 i64)
    (br 0 (block (result i32 i64) (call $dummy) (i32.const 1) (i64.const 2)))
  )

  (func (export "break-br_if-num-num") (param i32) (result i32 i64)
    (drop (drop (br_if 0 (i32.const 50) (i64.const 51) (local.get 0))))
    (i32.const 51) (i64.const 52)
  )

  (func (export "break-br_table-num-num") (param i32) (result i32 i64)
    (br_table 0 0 (i32.const 50) (i64.const 51) (local.get 0))
    (i32.const 51) (i64.const 52)
  )

  (func (export "break-br_table-nested-num-num") (param i32) (result i32 i32)
    (i32.add
      (block (result i32 i32)
        (br_table 0 1 0 (i32.const 50) (i32.const 51) (local.get 0))
        (i32.const 51) (i32.const -3)
      )
    )
    (i32.const 52)
  )

  (func (export "large-sig")
    (param i32 i64 f32 f32 i32 f64 f32 i32 i32 i32 f32 f64 f64 f64 i32 i32 f32)
    (result f64 f32 i32 i32 i32 i64 f32 i32 i32 f32 f64 f64 i32 f32 i32 f64)
    (local.get 5)
    (local.get 2)
    (local.get 0)
    (local.get 8)
    (local.get 7)
    (local.get 1)
    (local.get 3)
    (local.get 9)
    (local.get 4)
    (local.get 6)
    (local.get 13)
    (local.get 11)
    (local.get 15)
    (local.get 16)
    (local.get 14)
    (local.get 12)
  )
)
