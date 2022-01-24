
(module
  (memory 1)
    (func (export "i32_align_switch") (param i32 i32) (result i32)
    (local i32 i32)
    (local.set 2 (i32.const 10))
    (if (i32.eq (local.get 1) (i32.const 0))
      (then
        (i32.store8 (i32.const 0) (local.get 2))
        (local.set 3 (i32.load8_s (i32.const 0)))
      )
    )
    (if (i32.eq (local.get 1) (i32.const 1))
      (then
        (i32.store8 align=1 (i32.const 0) (local.get 2))
        (local.set 3 (i32.load8_s align=1 (i32.const 0)))
      )
    )
    (local.get 3)
  )
)
