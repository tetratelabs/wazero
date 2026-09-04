(module
  (type $s (struct (field i32)))

  (func (export "allocate_many")
    (local $i i32)
    (loop $l
      (drop (struct.new $s (local.get $i)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br_if $l (i32.lt_u (local.get $i) (i32.const 100000))))))
