(module
  (memory (export "memory") 1)

  ;; Load the i32 value at the offset 32, decrement it, and store it back at the same position
  ;; until the value becomes zero.
  (func (export "i32")
    (loop
      i32.const 32
      (i32.load align=1 (i32.const 32))
      i32.const 1
      i32.sub
      i32.store
      (br_if 1 (i32.eqz (i32.load align=1 (i32.const 32)))) ;; exit.
      (br 0) ;; continue loop.
    )
  )

  ;; Load the i64 value at the offset 64, decrement it, and store it back at the same position
  ;; until the value becomes zero.
  (func (export "i64")
    (loop
      i32.const 64
      (i64.load align=1 (i32.const 64))
      i64.const 1
      i64.sub
      i64.store
      (br_if 1 (i64.eqz (i64.load align=1 (i32.const 64)))) ;; exit.
      (br 0) ;; continue loop.
    )
  )
)
