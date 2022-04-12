;; This file includes changes to test/core/loop.wast from the commit that added "multi-value" support.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      --debug-names loop.wat
;;
;; See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
(module $loop.wast

;; preconditions
  (func $dummy)

;; changes
 (func (export "multi") (result i32)
    (loop (call $dummy) (call $dummy) (call $dummy) (call $dummy))
    (loop (result i32) (call $dummy) (call $dummy) (i32.const 8) (call $dummy))
    (drop)
    (loop (result i32 i64 i32)
      (call $dummy) (call $dummy) (call $dummy) (i32.const 8) (call $dummy)
      (call $dummy) (call $dummy) (call $dummy) (i64.const 7) (call $dummy)
      (call $dummy) (call $dummy) (call $dummy) (i32.const 9) (call $dummy)
    )
    (drop) (drop)
  )

  (func (export "as-binary-operands") (result i32)
    (i32.mul
      (loop (result i32 i32)
        (call $dummy) (i32.const 3) (call $dummy) (i32.const 4)
      )
    )
  )

  (func (export "as-compare-operands") (result i32)
    (f32.gt
      (loop (result f32 f32)
        (call $dummy) (f32.const 3) (call $dummy) (f32.const 3)
      )
    )
  )

  (func (export "as-mixed-operands") (result i32)
    (loop (result i32 i32)
      (call $dummy) (i32.const 3) (call $dummy) (i32.const 4)
    )
    (i32.const 5)
    (i32.add)
    (i32.mul)
  )

  (func (export "break-value") (result i32)
    (block (result i32)
      (i32.const 0)
      (loop (param i32)
        (block (br 2 (i32.const 18)))
        (br 0 (i32.const 20))
      )
      (i32.const 19)
    )
  )

  (func (export "break-multi-value") (result i32 i32 i64)
    (block (result i32 i32 i64)
      (i32.const 0) (i32.const 0) (i64.const 0)
      (loop (param i32 i32 i64)
        (block (br 2 (i32.const 18) (i32.const -18) (i64.const 18)))
        (br 0 (i32.const 20) (i32.const -20) (i64.const 20))
      )
      (i32.const 19) (i32.const -19) (i64.const 19)
    )
  )

  (func (export "param") (result i32)
    (i32.const 1)
    (loop (param i32) (result i32)
      (i32.const 2)
      (i32.add)
    )
  )

  (func (export "params") (result i32)
    (i32.const 1)
    (i32.const 2)
    (loop (param i32 i32) (result i32)
      (i32.add)
    )
  )

  (func (export "params-id") (result i32)
    (i32.const 1)
    (i32.const 2)
    (loop (param i32 i32) (result i32 i32))
    (i32.add)
  )

  (func (export "param-break") (result i32)
    (local $x i32)
    (i32.const 1)
    (loop (param i32) (result i32)
      (i32.const 4)
      (i32.add)
      (local.tee $x)
      (local.get $x)
      (i32.const 10)
      (i32.lt_u)
      (br_if 0)
    )
  )

  (func (export "params-break") (result i32)
    (local $x i32)
    (i32.const 1)
    (i32.const 2)
    (loop (param i32 i32) (result i32)
      (i32.add)
      (local.tee $x)
      (i32.const 3)
      (local.get $x)
      (i32.const 10)
      (i32.lt_u)
      (br_if 0)
      (drop)
    )
  )

  (func (export "params-id-break") (result i32)
    (local $x i32)
    (local.set $x (i32.const 0))
    (i32.const 1)
    (i32.const 2)
    (loop (param i32 i32) (result i32 i32)
      (local.set $x (i32.add (local.get $x) (i32.const 1)))
      (br_if 0 (i32.lt_u (local.get $x) (i32.const 10)))
    )
    (i32.add)
  )

  (type $block-sig-1 (func))
  (type $block-sig-2 (func (result i32)))
  (type $block-sig-3 (func (param $x i32)))
  (type $block-sig-4 (func (param i32 f64 i32) (result i32 f64 i32)))

  (func (export "type-use")
    (loop (type $block-sig-1))
    (loop (type $block-sig-2) (i32.const 0))
    (loop (type $block-sig-3) (drop))
    (i32.const 0) (f64.const 0) (i32.const 0)
    (loop (type $block-sig-4))
    (drop) (drop) (drop)
    (loop (type $block-sig-2) (result i32) (i32.const 0))
    (loop (type $block-sig-3) (param i32) (drop))
    (i32.const 0) (f64.const 0) (i32.const 0)
    (loop (type $block-sig-4)
      (param i32) (param f64 i32) (result i32 f64) (result i32)
    )
    (drop) (drop) (drop)
  )
)
