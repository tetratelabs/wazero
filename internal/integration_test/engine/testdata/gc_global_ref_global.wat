;; Minimal repro: a non-imported global's const expression references a
;; previously defined (non-imported) global via global.get.
;; This is valid under GC / extended-const but was rejected because
;; validation only exposed imported globals to const expression evaluation.
(module
  (type $point (struct (field i32) (field i32)))

  (global $a (ref $point) (struct.new $point (i32.const 10) (i32.const 20)))
  (global $b (ref $point) (global.get $a))

  (func (export "get_b_x") (result i32)
    (struct.get $point 0 (global.get $b))
  )
)
