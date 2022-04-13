;; This file includes changes to test/core/call.wast from the commit that added "multi-value" support.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      --debug-names call.wat
;;
;; See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
(module $call.wast

  (func $const-i32-i64 (result i32 i64) (i32.const 0x132) (i64.const 0x164))

  (func $id-i32-f64 (param i32 f64) (result i32 f64)
    (local.get 0) (local.get 1)
  )

  (func $swap-i32-i32 (param i32 i32) (result i32 i32)
    (local.get 1) (local.get 0)
  )

  (func $swap-f32-f64 (param f32 f64) (result f64 f32)
    (local.get 1) (local.get 0)
  )

  (func $swap-f64-i32 (param f64 i32) (result i32 f64)
    (local.get 1) (local.get 0)
  )

  (func (export "type-i32-i64") (result i32 i64) (call $const-i32-i64))

  (func (export "type-all-i32-f64") (result i32 f64)
    (call $id-i32-f64 (i32.const 32) (f64.const 1.64))
  )

  (func (export "type-all-i32-i32") (result i32 i32)
    (call $swap-i32-i32 (i32.const 1) (i32.const 2))
  )

  (func (export "type-all-f32-f64") (result f64 f32)
    (call $swap-f32-f64 (f32.const 1) (f64.const 2))
  )

  (func (export "type-all-f64-i32") (result i32 f64)
    (call $swap-f64-i32 (f64.const 1) (i32.const 2))
  )

  (func (export "as-binary-all-operands") (result i32)
    (i32.add (call $swap-i32-i32 (i32.const 3) (i32.const 4)))
  )

  (func (export "as-mixed-operands") (result i32)
    (call $swap-i32-i32 (i32.const 3) (i32.const 4))
    (i32.const 5)
    (i32.add)
    (i32.mul)
  )

  (func (export "as-call-all-operands") (result i32 i32)
    (call $swap-i32-i32 (call $swap-i32-i32 (i32.const 3) (i32.const 4)))
  )
)
