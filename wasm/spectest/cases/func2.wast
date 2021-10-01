;; https://github.com/WebAssembly/spec/blob/d404d096bcdbe7f663bd6b2384dd65c8023b8312/test/core/func.wast

(module
  (type $proc (func (result i32)))
  (type $sig (func (param i32) (result i32)))

  (func (export "f") (type $sig)
    (local $var i32)
    (local.get $var)
  )

  (func $g (type $sig)
    (local $var i32)
    (local.get $var)
  )
  (func (export "g") (type $sig)
    (call $g (local.get 0))
  )

  (func (export "p") (type $proc)
    (local $var i32)
    (local.set 0 (i32.const 42))
    (local.get $var)
  )
)

;; (assert_return (invoke "f" (i32.const 42)) (i32.const 0))
;; (assert_return (invoke "g" (i32.const 42)) (i32.const 0))
;; (assert_return (invoke "p") (i32.const 42))

