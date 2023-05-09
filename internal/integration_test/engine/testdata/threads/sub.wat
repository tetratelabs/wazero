(module
  (memory 1 1 shared)

  (func (export "run32")
    (i32.atomic.rmw.sub (i32.const 0) (i32.const 1))
    (drop)
  )

  (func (export "run64")
    (i64.atomic.rmw.sub (i32.const 0) (i64.const 1))
    (drop)
  )

  (func (export "run32_8")
    (i32.atomic.rmw8.sub_u (i32.const 0) (i32.const 1))
    (drop)
  )

  (func (export "run32_16")
    (i32.atomic.rmw16.sub_u (i32.const 0) (i32.const 1))
    (drop)
  )

  (func (export "run64_8")
    (i64.atomic.rmw8.sub_u (i32.const 0) (i64.const 1))
    (drop)
  )

  (func (export "run64_16")
    (i64.atomic.rmw16.sub_u (i32.const 0) (i64.const 1))
    (drop)
  )

  (func (export "run64_32")
    (i64.atomic.rmw32.sub_u (i32.const 0) (i64.const 1))
    (drop)
  )
)
