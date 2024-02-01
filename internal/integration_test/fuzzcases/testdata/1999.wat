(module
  (type (;0;) (func (param externref)))
  (func (;0;) (type 0) (param externref)
    i32.const 2147483647
    i32.const -11579569
    i32.div_u
    f64.convert_i32_s
    i64.trunc_f64_s
    f64.convert_i64_u
    i64.trunc_f64_s
    unreachable
  )
  (memory (;0;) 0 8)
  (export "" (func 0))
  (export "1" (memory 0))
)
