(module
  (type (;0;) (func (param f64 v128 i32) (result i64)))
  (func (;0;) (type 0) (param f64 v128 i32) (result i64)
    (local v128 v128 f64 f64)
    local.get 1
    i64x2.abs
    v128.const i32x4 0xffff6824 0xffff6262 0xffffffff 0x363636ff
    f64x2.floor
    local.tee 3
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 3
    local.get 3
    f64x2.eq
    v128.bitselect
    f64x2.floor
    local.tee 4
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 4
    local.get 4
    f64x2.eq
    v128.bitselect
    i8x16.le_s
    local.get 0
    f64.sqrt
    local.tee 5
    f64.const nan (;=NaN;)
    local.get 5
    local.get 5
    f64.eq
    select
    f64.sqrt
    local.tee 6
    f64.const nan (;=NaN;)
    local.get 6
    local.get 6
    f64.eq
    select
    local.tee 0
    i64.trunc_sat_f64_u
    i64.const 3906369100484640767
    i64.le_u
    f32.convert_i32_s
    i64.trunc_f32_s
    i64.extend16_s
    f64.reinterpret_i64
    i32.trunc_f64_u
    i32.const 50282532
    global.get 0
    i32.xor
    global.set 0
    global.get 0
    i32.xor
    global.set 0
    global.get 1
    v128.xor
    global.set 1
    i64.const 36
  )
  (global (;0;) (mut i32) i32.const 0)
  (global (;1;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;2;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (global 0))
  (export "2" (global 1))
)
