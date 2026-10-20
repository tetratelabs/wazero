(module
  (type $inner (struct (field i32)))
  (type $outer (struct (field (ref null $inner))))
  (global $g (mut (ref null $outer)) (ref.null none))

  (func (export "setup")
    (global.set $g
      (struct.new $outer
        (struct.new $inner (i32.const 42)))))

  (func (export "read_inner") (result i32)
    (struct.get $inner 0
      (struct.get $outer 0
        (global.get $g)))))
