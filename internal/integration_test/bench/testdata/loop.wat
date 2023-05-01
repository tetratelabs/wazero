(module
  (func (export "loop")
    (local $i i32)
    (loop
      local.get $i
      i32.const 1
      i32.add
      local.set $i

      local.get $i
      i32.const 10000
      i32.lt_s
      br_if 0
    )
  )
)
