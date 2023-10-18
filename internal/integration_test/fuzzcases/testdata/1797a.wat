(module
  (type (;0;) (func (param i32) (result i32 i32 i32)))
  (type (;1;) (func (param i32 i32)))
  (type (;2;) (func (result i32)))
  (type (;3;) (func))
  (func (;0;) (type 2) (result i32)
    (local v128)
    i32.const -353703190
    i8x16.splat
    f32x4.floor
    local.tee 0
    v128.const i32x4 0x7fc00000 0x7fc00000 0x7fc00000 0x7fc00000
    local.get 0
    local.get 0
    f32x4.eq
    v128.bitselect
    i64x2.extend_low_i32x4_s
    i64x2.neg
    f64x2.extract_lane 0
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    f64.convert_i32_u
    i32.trunc_f64_s
    i64.const -709973716250918950
    i32.wrap_i64
    i8x16.splat
    i32.const -353703190
    i8x16.shl
    f64x2.neg
    global.get 0
    v128.xor
    global.set 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (global 0))
)
