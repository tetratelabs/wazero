(module
  (memory 1 1 shared)

  (func (export "run32")
    (i32.atomic.rmw.xor (i32.const 0) (i32.const 0xFFFFFFFF))
    (drop)
  )

  (func (export "run64")
    (i64.atomic.rmw.xor (i32.const 0) (i64.const 0xFFFFFFFFFFFFFFFF))
    (drop)
  )

  (func (export "run32_8")
    (i32.atomic.rmw8.xor_u (i32.const 0) (i32.const 0xFFFFFFFF))
    (drop)
  )

  (func (export "run32_16")
    (i32.atomic.rmw16.xor_u (i32.const 0) (i32.const 0xFFFFFFFF))
    (drop)
  )

  (func (export "run64_8")
    (i64.atomic.rmw8.xor_u (i32.const 0) (i64.const 0xFFFFFFFFFFFFFFFF))
    (drop)
  )

  (func (export "run64_16")
    (i64.atomic.rmw16.xor_u (i32.const 0) (i64.const 0xFFFFFFFFFFFFFFFF))
    (drop)
  )

  (func (export "run64_32")
    (i64.atomic.rmw32.xor_u (i32.const 0) (i64.const 0xFFFFFFFFFFFFFFFF))
    (drop)
  )
)
