(module
  (type (;0;) (func (result v128)))
  (func (;0;) (type 0) (result v128)
    v128.const i32x4 0x00000000 0x00000000 0x00000000 0x7fc00000
    v128.const i32x4 0x00000000 0x00000000 0x00000000 0x7ff80000
    f32x4.mul
  )
  (export "" (func 0))
)
