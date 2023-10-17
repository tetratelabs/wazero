(module
  (type (;0;) (func (param f64 f64)))
  (func (;0;) (type 0) (param f64 f64)
    (local externref i64 v128)
    v128.const i32x4 0x67676767 0x67676767 0xa9676767 0x67676767
    f32x4.ceil
    local.tee 4
    v128.const i32x4 0x7fc00000 0x7fc00000 0x7fc00000 0x7fc00000
    local.get 4
    local.get 4
    f32x4.eq
    v128.bitselect
    v128.const i32x4 0x40bf0242 0xff89ff40 0x64ffffff 0x96966464
    i8x16.ne
    global.get 0
    v128.xor
    global.set 0
  )
  (memory (;0;) 4 4)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (memory 0))
  (export "zz" (global 0))
)
