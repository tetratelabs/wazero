(module
  (type (;0;) (func (param funcref f64)))
  (type (;1;) (func (result v128 f32 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i64 f64 f32 f64 f64 f64 f64 f64)))
  (type (;2;) (func))
  (func (;0;) (type 0) (param funcref f64)
    global.get 4
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 4
    i32.const 1
    i32.sub
    global.set 4
  )
  (func (;1;) (type 1) (result v128 f32 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i64 f64 f32 f64 f64 f64 f64 f64)
    v128.const i32x4 0x0aa6a6a6 0x92926e6e 0x6e6e6e6e 0x6e6effff
    f32.const -0x1.dcdcdcp+57 (;=-268449850000000000;)
    v128.const i32x4 0x6e6e6e11 0x6e6e6e6e 0x6e6e6e6e 0xff6e6e6e
    v128.const i32x4 0x0111802c 0x80000000 0xd0ffffff 0x6e00d0d0
    v128.const i32x4 0x6e6e6e6e 0x6e6e6e6e 0x6e6e6e6e 0x6e6e6e6e
    v128.const i32x4 0x6e6e6e6e 0x6e6e6e6e 0x6e6e6e6e 0x6e6e6e6e
    v128.const i32x4 0x00000015 0xd0d0d0ff 0x6e4b0a00 0x6eee6e6e
    v128.const i32x4 0xd0d06e6e 0x6e4b0a00 0x6eee6e6e 0x6e6e6e6e
    v128.const i32x4 0x6e6e6e6e 0x6e6e6e6e 0x6e6e6e6e 0x6e6e6e6e
    v128.const i32x4 0x6e6e6e6e 0x766e6e6e 0x00000076 0xffff0000
    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffcd
    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xe3e3e3ff
    i64.const -2025524719207062557
    f64.const -nan:0xfffffffffffe3 (;=NaN;)
    f32.const -0x1.fffffep+72 (;=-9444732400000000000000;)
    f64.const -nan:0xfe3e3e3e3e3e3 (;=NaN;)
    f64.const -0x1.fffffff6e6e6ep+576 (;=-494660802422288500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
    f64.const 0x1.fe3e3e3e3e3e3p+752 (;=47216516855985470000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
    f64.const 0x1.e6e6e6e6e6e6ep+743 (;=88001147761747390000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
    f64.const -0x1.df7ffff246e6ep+1012 (;=-82206140203098770000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
  )
  (func (;2;) (type 1) (result v128 f32 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i64 f64 f32 f64 f64 f64 f64 f64)
    global.get 4
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 4
    i32.const 1
    i32.sub
    global.set 4
    unreachable
    unreachable
  )
  (func (;3;) (type 0) (param funcref f64)
    (local f64)
    call 1
    f64.ne
    i64.extend_i32_u
    loop (result v128 f32 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i64 f64 f32 f64 f64 f64 f64 f64)  ;; label = @1
      call 1
      f64.max
      local.tee 2
      f64.const nan (;=NaN;)
      local.get 2
      local.get 2
      f64.eq
      select
      f64.copysign
      f64.copysign
      f64.copysign
      i64.const 7967591693768617582
      loop (result v128 f32 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i64 f64 f32 f64 f64 f64 f64 f64)  ;; label = @2
        call 1
        call 1
        f64.copysign
        f64.copysign
        i64.reinterpret_f64
        global.get 0
        i64.xor
        global.set 0
        i64.reinterpret_f64
        global.get 0
        i64.xor
        global.set 0
        i64.reinterpret_f64
        global.get 0
        i64.xor
        global.set 0
        i32.reinterpret_f32
        global.get 1
        i32.xor
        global.set 1
        i64.reinterpret_f64
        global.get 0
        i64.xor
        global.set 0
        drop
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        global.get 2
        v128.xor
        global.set 2
        i32.reinterpret_f32
        global.get 1
        i32.xor
        global.set 1
        global.get 2
        v128.xor
        global.set 2
      end
      i64.reinterpret_f64
      global.get 0
      i64.xor
      global.set 0
      i64.reinterpret_f64
      global.get 0
      i64.xor
      global.set 0
      i64.reinterpret_f64
      global.get 0
      i64.xor
      global.set 0
      i64.reinterpret_f64
      global.get 0
      i64.xor
      global.set 0
      i64.reinterpret_f64
      global.get 0
      i64.xor
      global.set 0
      i32.reinterpret_f32
      global.get 1
      i32.xor
      global.set 1
      i64.reinterpret_f64
      global.get 0
      i64.xor
      global.set 0
      global.get 3
      i64.xor
      global.set 3
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      global.get 2
      v128.xor
      global.set 2
      i32.reinterpret_f32
      global.get 1
      i32.xor
      global.set 1
      global.get 2
      v128.xor
      global.set 2
      drop
      f64.const 0x1.0d0d0d0ffp-863 (;=0.000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000017088673646626603;)
      f64.const 0x1.e6eee6e6e6e4bp+743 (;=88006795789664450000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
      f64.const 0x1.4dcdcp-1055 (;=0.00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000337758;)
      f64.const 0x1.e6e6e6e6ep+743 (;=88001147761456950000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
    end
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i32.reinterpret_f32
    global.get 1
    i32.xor
    global.set 1
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    global.get 3
    i64.xor
    global.set 3
    global.get 2
    v128.xor
    global.set 2
    drop
    drop
    drop
    drop
    drop
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    i32.reinterpret_f32
    global.get 1
    i32.xor
    global.set 1
    global.get 2
    v128.xor
    global.set 2
    global.get 3
    i64.xor
    global.set 3
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    i32.reinterpret_f32
    global.get 1
    i32.xor
    global.set 1
    i64.reinterpret_f64
    global.get 0
    i64.xor
    global.set 0
    global.get 3
    i64.xor
    global.set 3
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    global.get 2
    v128.xor
    global.set 2
    i32.reinterpret_f32
    global.get 1
    i32.xor
    global.set 1
    global.get 2
    v128.xor
    global.set 2
  )
  (func (;4;) (type 0) (param funcref f64)
    global.get 4
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 4
    i32.const 1
    i32.sub
    global.set 4
  )
  (table (;0;) 0 755 externref)
  (table (;1;) 1000 1000 funcref)
  (global (;0;) (mut i64) i64.const 0)
  (global (;1;) (mut i32) i32.const 0)
  (global (;2;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;3;) (mut i64) i64.const 0)
  (global (;4;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (func 1))
  (export "2" (func 2))
  (export "3" (func 3))
  (export "4" (func 4))
  (export "5" (table 0))
  (export "6" (table 1))
  (export "7" (global 0))
  (export "8" (global 1))
  (export "9" (global 2))
  (export "10" (global 3))
  (elem (;0;) externref)
)
