(module
  (type (;0;) (func (param i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32) (result i32 i32 i32 i32)))
  (func (;0;) (type 0) (param i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32) (result i32 i32 i32 i32)
    global.get 2
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 2
    i32.const 1
    i32.sub
    global.set 2
    v128.const i32x4 0xff8000ff 0x000000ff 0x000fc5ff 0xffffff01
    local.get 15
    i32.extend8_s
    i32.extend8_s
    i32.extend8_s
    local.get 15
    i32.eq
    i32.eqz
    i32.extend8_s
    i32.extend8_s
    unreachable
    i8x16.replace_lane 4
    i64.const -585884069744
    data.drop 0
    data.drop 0
    i32.const 1869573999
    i16x8.splat
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    v128.const i32x4 0x45000000 0x0000003a 0x00000000 0x2400f75d
    i8x16.shuffle 9 9 9 9 9 9 9 9 9 13 9 9 9 9 9 9
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f64x2.abs
    f32x4.abs
    i32x4.extend_low_i16x8_s
    unreachable
  )
  (func (;1;) (type 0) (param i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32) (result i32 i32 i32 i32)
    global.get 2
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 2
    i32.const 1
    i32.sub
    global.set 2
    block  ;; label = @1
      f64.const -0x1.a246969696969p+1016 (;=-1147356289394718200000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      br 0 (;@1;)
      v128.const i32x4 0x69696969 0x69696969 0x69696969 0x45696969
      global.get 0
      v128.xor
      global.set 0
      i64.reinterpret_f64
      global.get 1
      i64.xor
      global.set 1
    end
    i32.const -867893179
    i32.const -858993444
    i32.const -13312
    i32.const 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i64) i64.const 0)
  (global (;2;) (mut i32) i32.const 1000)
  (export "~zz\00E1E\00EE\00$" (func 0))
  (export "" (func 1))
  (export "2" (global 0))
  (export "3" (global 1))
  (data (;0;) "\f7\00\ff\ff\ff\0e\00")
)