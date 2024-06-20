(module
  (type (;0;) (func (param f64)))
  (func (;0;) (type 0) (param f64)
    (local v128 v128 v128)
    v128.const i32x4 0x8a8a8aff 0x8a8a8aaa 0xff458a8a 0x8affffff
    f32x4.ceil
    global.set 0
    global.get 0
    v128.const i32x4 0x8a8a8aff 0x8a8a8aaa 0xff458a8a 0x8affffff
    f32x4.ceil
    global.set 1
    global.get 1
    i16x8.q15mulr_sat_s
    global.set 2
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 0)
  (export "" (func 0))
)