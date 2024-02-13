(module
  (type (;0;) (func (param i32) (result v128)))
  (func (;0;) (type 0) (param i32) (result v128)
    i32.const 0
    if ;; label = @1
      unreachable
    end
    v128.const i32x4 0x4a4a4a4a 0x4a4a4a4a 0x4a4a4a4a 0x4a4a4a4a
    f32x4.convert_i32x4_u
    i16x8.extadd_pairwise_i8x16_u
  )
  (export "" (func 0))
)
