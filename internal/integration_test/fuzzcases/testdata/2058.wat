(module
  (type (;0;) (func (param i32 i32 i32)))
  (func (;0;) (type 0) (param i32 i32 i32)
    global.get 0
    v128.const i32x4 0xa0a00000 0x1060a0a0 0xffffffff 0xa0ffffff
    i64x2.lt_s
    global.set 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 0)
  (export "" (func 0))
)
