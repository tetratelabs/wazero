;; Minimal repro: ref.null with abstract bottom type (none) in a global
;; initializer for a concrete struct type. The validator must accept
;; nullref as a subtype of (ref null $struct_type).
(module
  (type $point (struct (field i32) (field i32)))

  (global $g (mut (ref null $point)) (ref.null none))

  (func (export "set_and_get") (result i32)
    (global.set $g (struct.new $point (i32.const 10) (i32.const 20)))
    (struct.get $point 0 (global.get $g))
  )
)
