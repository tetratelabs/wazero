;; This file includes changes to test/core/if.wast from the commit that added "multi-value" support.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      --debug-names if.wat
;;
;; See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
(module $if.wast
;; preconditions
  (func $dummy)

;; changes
  (func (export "multi") (param i32) (result i32 i32)
    (if (local.get 0) (then (call $dummy) (call $dummy) (call $dummy)))
    (if (local.get 0) (then) (else (call $dummy) (call $dummy) (call $dummy)))
    (if (result i32) (local.get 0)
      (then (call $dummy) (call $dummy) (i32.const 8) (call $dummy))
      (else (call $dummy) (call $dummy) (i32.const 9) (call $dummy))
    )
    (if (result i32 i64 i32) (local.get 0)
      (then
        (call $dummy) (call $dummy) (i32.const 1) (call $dummy)
        (call $dummy) (call $dummy) (i64.const 2) (call $dummy)
        (call $dummy) (call $dummy) (i32.const 3) (call $dummy)
      )
      (else
        (call $dummy) (call $dummy) (i32.const -1) (call $dummy)
        (call $dummy) (call $dummy) (i64.const -2) (call $dummy)
        (call $dummy) (call $dummy) (i32.const -3) (call $dummy)
      )
    )
    (drop) (drop)
  )

  (func (export "as-binary-operands") (param i32) (result i32)
    (i32.mul
      (if (result i32 i32) (local.get 0)
        (then (call $dummy) (i32.const 3) (call $dummy) (i32.const 4))
        (else (call $dummy) (i32.const 3) (call $dummy) (i32.const -4))
      )
    )
  )

  (func (export "as-compare-operands") (param i32) (result i32)
    (f32.gt
      (if (result f32 f32) (local.get 0)
        (then (call $dummy) (f32.const 3) (call $dummy) (f32.const 3))
        (else (call $dummy) (f32.const -2) (call $dummy) (f32.const -3))
      )
    )
  )

  (func (export "as-mixed-operands") (param i32) (result i32)
    (if (result i32 i32) (local.get 0)
      (then (call $dummy) (i32.const 3) (call $dummy) (i32.const 4))
      (else (call $dummy) (i32.const -3) (call $dummy) (i32.const -4))
    )
    (i32.const 5)
    (i32.add)
    (i32.mul)
  )

  (func (export "break-multi-value") (param i32) (result i32 i32 i64)
    (if (result i32 i32 i64) (local.get 0)
      (then
        (br 0 (i32.const 18) (i32.const -18) (i64.const 18))
        (i32.const 19) (i32.const -19) (i64.const 19)
      )
      (else
        (br 0 (i32.const -18) (i32.const 18) (i64.const -18))
        (i32.const -19) (i32.const 19) (i64.const -19)
      )
    )
  )

  (func (export "param") (param i32) (result i32)
    (i32.const 1)
    (if (param i32) (result i32) (local.get 0)
      (then (i32.const 2) (i32.add))
      (else (i32.const -2) (i32.add))
    )
  )

  (func (export "params") (param i32) (result i32)
    (i32.const 1)
    (i32.const 2)
    (if (param i32 i32) (result i32) (local.get 0)
      (then (i32.add))
      (else (i32.sub))
    )
  )

  (func (export "params-id") (param i32) (result i32)
    (i32.const 1)
    (i32.const 2)
    (if (param i32 i32) (result i32 i32) (local.get 0) (then))
    (i32.add)
  )

  (func (export "param-break") (param i32) (result i32)
    (i32.const 1)
    (if (param i32) (result i32) (local.get 0)
      (then (i32.const 2) (i32.add) (br 0))
      (else (i32.const -2) (i32.add) (br 0))
    )
  )

  (func (export "params-break") (param i32) (result i32)
    (i32.const 1)
    (i32.const 2)
    (if (param i32 i32) (result i32) (local.get 0)
      (then (i32.add) (br 0))
      (else (i32.sub) (br 0))
    )
  )

  (func (export "params-id-break") (param i32) (result i32)
    (i32.const 1)
    (i32.const 2)
    (if (param i32 i32) (result i32 i32) (local.get 0) (then (br 0)))
    (i32.add)
  )

  (func $add64_u_with_carry (export "add64_u_with_carry")
    (param $i i64) (param $j i64) (param $c i32) (result i64 i32)
    (local $k i64)
    (local.set $k
      (i64.add
        (i64.add (local.get $i) (local.get $j))
        (i64.extend_i32_u (local.get $c))
      )
    )
    (return (local.get $k) (i64.lt_u (local.get $k) (local.get $i)))
  )

  (func $add64_u_saturated (export "add64_u_saturated")
    (param i64 i64) (result i64)
    (call $add64_u_with_carry (local.get 0) (local.get 1) (i32.const 0))
    (if (param i64) (result i64)
      (then (drop) (i64.const -1))
    )
  )

  (type $block-sig-1 (func))
  (type $block-sig-2 (func (result i32)))
  (type $block-sig-3 (func (param $x i32)))
  (type $block-sig-4 (func (param i32 f64 i32) (result i32 f64 i32)))

  (func (export "type-use")
    (if (type $block-sig-1) (i32.const 1) (then))
    (if (type $block-sig-2) (i32.const 1)
      (then (i32.const 0)) (else (i32.const 2))
    )
    (if (type $block-sig-3) (i32.const 1) (then (drop)) (else (drop)))
    (i32.const 0) (f64.const 0) (i32.const 0)
    (if (type $block-sig-4) (i32.const 1) (then))
    (drop) (drop) (drop)
    (if (type $block-sig-2) (result i32) (i32.const 1)
      (then (i32.const 0)) (else (i32.const 2))
    )
    (if (type $block-sig-3) (param i32) (i32.const 1)
      (then (drop)) (else (drop))
    )
    (i32.const 0) (f64.const 0) (i32.const 0)
    (if (type $block-sig-4)
      (param i32) (param f64 i32) (result i32 f64) (result i32)
      (i32.const 1) (then)
    )
    (drop) (drop) (drop)
  )
)
