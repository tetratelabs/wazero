(module
  (memory (export "memory") 1)

  ;; Load the i32 value at the offset 32, increment it, and store it back at the same position.
  (func (export "i32")
    i32.const 32
    i32.const 32
    i32.load align=1
    i32.const 1
    i32.add
    i32.store
  )

  ;; Load the i64 value at the offset 64, increment it, and store it back at the same position.
  (func (export "i64")
    i32.const 64
    i32.const 64
    i64.load align=1
    i64.const 1
    i64.add
    i64.store
  )
)
