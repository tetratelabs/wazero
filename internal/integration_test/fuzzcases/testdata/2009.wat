(module
  (func (result i32)
    f32.const -0x1.e1d94ap+60 (;=-2170054000000000000;)
    f32.const 0x1p+0 (;=1;)
    f32.ge
    f32.convert_i32_u
    f32.const -0x1.f74b1p+36 (;=-135101740000;)
    f32.const -0x1.f74b1p+36 (;=-135101740000;)
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.const 0x1.28608ep-26 (;=0.000000017251422;)
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    i32.const 0
    f32.convert_i32_u
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.const 0x1p+0 (;=1;)
    f32.ge
    f32.convert_i32_u
    f32.const 0x1p+0 (;=1;)
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.const 0.0
    f32.ge
    f32.convert_i32_u
    f32.trunc
    global.set 2
    global.get 2
    f32.const 0x1p+0 (;=1;)
    i32.const 1
    select
    f32.const 0.0
    f32.copysign
    global.set 1
    global.get 1
    i64.trunc_f32_s
    global.set 0
    i32.reinterpret_f32
  )
  (global (;0;) (mut i64) i64.const 0)
  (global (;1;) (mut f32) f32.const 2)
  (global (;2;) (mut f32) f32.const 2)
  (global (;1;) (mut i32) i32.const 2) ;; dummy
  (export "" (func 0))
)
