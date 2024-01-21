(module
  (type (;0;) (func))
  (type (;1;) (func (param f64 f64 f64) (result externref f64 i64)))
  (type (;2;) (func (param f64 f64 f64 i64 f64 funcref)))
  (func (;0;) (type 0)
    (local f32)
    v128.const i32x4 0x23808080 0x23232327 0xffffffff 0xffffffff
    ref.null func
    i32.const 1549556771
    i16x8.splat
    i64x2.all_true
    i64.extend_i32_u
    f32.const 0x1.fe49fep-55 (;=0.000000000000000055325648;)
    f32.nearest
    local.tee 0
    f32.const nan (;=NaN;)
    local.get 0
    local.get 0
    f32.eq
    select
    i32.reinterpret_f32
    global.get 0
    i32.xor
    global.set 0
    global.get 1
    i64.xor
    global.set 1
    drop
    global.get 2
    v128.xor
    global.set 2
  )
  (table (;0;) 26 510 funcref)
  (global (;0;) (mut i32) i32.const 0)
  (global (;1;) (mut i64) i64.const 0)
  (global (;2;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;3;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "\00\00\00\00" (table 0))
  (export "EEEE\02\00" (global 0))
  (export "3" (global 1))
  (export "4" (global 2))
)
