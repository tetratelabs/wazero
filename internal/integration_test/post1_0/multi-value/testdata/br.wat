;; This file includes changes to test/core/br.wast from the commit that added "multi-value" support.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      br.wat
;;
;; See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
(module $br.wast

;; preconditions
  (func $f (param i32 i32 i32) (result i32) (i32.const -1))
  (type $sig (func (param i32 i32 i32) (result i32)))
  (table funcref (elem $f))
  (memory 1)

;; changes
  (func (export "type-i32-i32") (block (drop (i32.add (br 0)))))
  (func (export "type-i64-i64") (block (drop (i64.add (br 0)))))
  (func (export "type-f32-f32") (block (drop (f32.add (br 0)))))
  (func (export "type-f64-f64") (block (drop (f64.add (br 0)))))

  (func (export "type-f64-f64-value") (result f64 f64)
    (block (result f64 f64)
      (f64.add (br 0 (f64.const 4) (f64.const 5))) (f64.const 6)
    )
  )

  (func (export "as-return-values") (result i32 i64)
    (i32.const 2)
    (block (result i64) (return (br 0 (i32.const 1) (i64.const 7))))
  )

  (func (export "as-select-all") (result i32)
    (block (result i32) (select (br 0 (i32.const 8))))
  )

  (func (export "as-call-all") (result i32)
    (block (result i32) (call $f (br 0 (i32.const 15))))
  )

  (func (export "as-call_indirect-all") (result i32)
    (block (result i32) (call_indirect (type $sig) (br 0 (i32.const 24))))
  )

  (func (export "as-store-both") (result i32)
    (block (result i32)
      (i64.store (br 0 (i32.const 32))) (i32.const -1)
    )
  )

  (func (export "as-storeN-both") (result i32)
    (block (result i32)
      (i64.store16 (br 0 (i32.const 34))) (i32.const -1)
    )
  )

  (func (export "as-binary-both") (result i32)
    (block (result i32) (i32.add (br 0 (i32.const 46))))
  )

  (func (export "as-compare-both") (result i32)
    (block (result i32) (f64.le (br 0 (i32.const 44))))
  )
)
