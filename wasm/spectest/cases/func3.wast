;; https://github.com/WebAssembly/spec/blob/d404d096bcdbe7f663bd6b2384dd65c8023b8312/test/core/func.wast

(module
  (type $sig (func))

  (func $empty-sig-1)  ;; should be assigned type $sig
  (func $complex-sig-1 (param f64 i64 f64 i64 f64 i64 f32 i32))
  (func $empty-sig-2)  ;; should be assigned type $sig
  (func $complex-sig-2 (param f64 i64 f64 i64 f64 i64 f32 i32))
  (func $complex-sig-3 (param f64 i64 f64 i64 f64 i64 f32 i32))
  (func $complex-sig-4 (param i64 i64 f64 i64 f64 i64 f32 i32))
  (func $complex-sig-5 (param i64 i64 f64 i64 f64 i64 f32 i32))

  (type $empty-sig-duplicate (func))
  (type $complex-sig-duplicate (func (param i64 i64 f64 i64 f64 i64 f32 i32)))
  (table funcref
    (elem
      $complex-sig-3 $empty-sig-2 $complex-sig-1 $complex-sig-3 $empty-sig-1
      $complex-sig-4 $complex-sig-5
    )
  )

  (func (export "signature-explicit-reused")
    (call_indirect (type $sig) (i32.const 1))
    (call_indirect (type $sig) (i32.const 4))
  )

  (func (export "signature-implicit-reused")
    ;; The implicit index 3 in this test depends on the function and
    ;; type definitions, and may need adapting if they change.
    (call_indirect (type 3)
      (f64.const 0) (i64.const 0) (f64.const 0) (i64.const 0)
      (f64.const 0) (i64.const 0) (f32.const 0) (i32.const 0)
      (i32.const 0)
    )
    (call_indirect (type 3)
      (f64.const 0) (i64.const 0) (f64.const 0) (i64.const 0)
      (f64.const 0) (i64.const 0) (f32.const 0) (i32.const 0)
      (i32.const 2)
    )
    (call_indirect (type 3)
      (f64.const 0) (i64.const 0) (f64.const 0) (i64.const 0)
      (f64.const 0) (i64.const 0) (f32.const 0) (i32.const 0)
      (i32.const 3)
    )
  )

  (func (export "signature-explicit-duplicate")
    (call_indirect (type $empty-sig-duplicate) (i32.const 1))
  )

  (func (export "signature-implicit-duplicate")
    (call_indirect (type $complex-sig-duplicate)
      (i64.const 0) (i64.const 0) (f64.const 0) (i64.const 0)
      (f64.const 0) (i64.const 0) (f32.const 0) (i32.const 0)
      (i32.const 5)
    )
    (call_indirect (type $complex-sig-duplicate)
      (i64.const 0) (i64.const 0) (f64.const 0) (i64.const 0)
      (f64.const 0) (i64.const 0) (f32.const 0) (i32.const 0)
      (i32.const 6)
    )
  )
)

;; (assert_return (invoke "signature-explicit-reused"))
;; (assert_return (invoke "signature-implicit-reused"))
;; (assert_return (invoke "signature-explicit-duplicate"))
;; (assert_return (invoke "signature-implicit-duplicate"))

