(module
  (func (;1;) (result v128 v128 f64 v128 v128 v128 i32 v128 v128 v128 v128 v128)
    v128.const i32x4 0xffffffff 0x0000ffff 0xfefff700 0xffffffff
    v128.const i32x4 0xffffffff 0xffffffff 0x24ffffff 0x10108240
    f64.const -0x1.3e3e3e3e3e3e3p+575 (;=-153732818170537500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000;)
    v128.const i32x4 0x02ffe3e3 0x00e3ff00 0x5151ffff 0x391b5151
    v128.const i32x4 0xe3e34000 0xff10e3e3 0xffff3aff 0x1082f1ff
    v128.const i32x4 0x45ffffff 0x103735c2 0x243a0010 0x0000517f
    i32.const 0
    v128.const i32x4 0xffffff00 0xffffff01 0xffffffff 0x505050ff
    v128.const i32x4 0x00001071 0x00ffffff 0x00000000 0x50505050
    v128.const i32x4 0x50505050 0xff51514e 0xd60001ff 0x10505050
    v128.const i32x4 0x50505050 0x50105050 0x4e505010 0x50105151
    v128.const i32x4 0x50505050 0xffdc1050 0xffffffff 0x000000ff
  )
  (func (;4;)
    (local i32)
    f64.const 0
    call 0
    i32x4.ge_u
    f64x2.pmin
    i16x8.narrow_i32x4_s
    f64x2.pmin
    memory.size
    call 0
    v128.any_true
    call 0
    v128.any_true
    call 0
    i16x8.gt_s
    v128.bitselect
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    i64.reinterpret_f64
    global.get 2
    i64.xor
    global.set 2
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    i64.reinterpret_f64
    global.get 2
    i64.xor
    global.set 2
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    drop
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    i64.reinterpret_f64
    global.get 2
    i64.xor
    global.set 2
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    global.get 0
    v128.xor
    global.set 0
    drop
    global.get 0
    v128.xor
    global.set 0
    i64.reinterpret_f64
    global.get 2
    i64.xor
    global.set 2
    global.get 0
    v128.xor
    global.set 0
    global.get 0
    v128.xor
    global.set 0
    drop
  )
  (table (;0;) 1000 1000 externref)
  (table (;1;) 1000 1000 externref)
  (memory (;0;) 0 0)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 0)
  (global (;2;) (mut i64) i64.const 0)
  (global (;3;) (mut i32) i32.const 1000)
  (export "" (func 1))
)
