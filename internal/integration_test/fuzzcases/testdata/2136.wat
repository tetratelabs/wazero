(module
  (type (;0;) (func (result f32)))
  (func (;0;) (type 0) (result f32)
    i32.const 0
    i32.load8_s
    f32.convert_i32_u
  )
  (memory (;0;) 3 8)
  (export "" (func 0))
  (data (;0;) (i32.const 0) "\f2")
)
