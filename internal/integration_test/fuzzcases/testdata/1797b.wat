(module
  (type (;0;) (func (param i32 i32 i32 i32 i32 i32)))
  (func (;0;) (type 0) (param i32 i32 i32 i32 i32 i32)
    v128.const i32x4 0xffff2f90 0x24ffffff 0x90d6240a 0xffff2f90
    f32x4.abs
    local.get 5
    i16x8.shr_u
    f32x4.abs
    global.get 0
    v128.xor
    global.set 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 1000)
  (export "\00\00\00\00\00" (func 0))
  (export "" (global 0))
)
