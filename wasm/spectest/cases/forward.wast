;; https://github.com/WebAssembly/spec/blob/d404d096bcdbe7f663bd6b2384dd65c8023b8312/test/core/forward.wast

(module
  (func $even (export "even") (param $n i32) (result i32)
    (if (result i32) (i32.eq (local.get $n) (i32.const 0))
      (then (i32.const 1))
      (else (call $odd (i32.sub (local.get $n) (i32.const 1))))
    )
  )

  (func $odd (export "odd") (param $n i32) (result i32)
    (if (result i32) (i32.eq (local.get $n) (i32.const 0))
      (then (i32.const 0))
      (else (call $even (i32.sub (local.get $n) (i32.const 1))))
    )
  )
)

;; (assert_return (invoke "even" (i32.const 13)) (i32.const 0))
;; (assert_return (invoke "even" (i32.const 20)) (i32.const 1))
;; (assert_return (invoke "odd" (i32.const 13)) (i32.const 1))
;; (assert_return (invoke "odd" (i32.const 20)) (i32.const 0))