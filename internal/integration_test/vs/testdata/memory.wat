(module
  (memory (export "memory") 1)

  (func (export "i32")
    i32.const 64
    i32.const 64
    i32.load align=1
    i32.const 1
    i32.add
    i32.store
  )

  (func (export "i64")
    i32.const 128
    i32.const 128
    i64.load align=1
    i64.const 1
    i64.add
    i64.store
  )
)
