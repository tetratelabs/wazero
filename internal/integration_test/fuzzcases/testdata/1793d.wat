(module
  (type (;0;) (func (result i64)))
  (func (;0;) (type 0) (result i64)
    (local f64 f64 f64 f64 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    global.get 2
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 2
    i32.const 1
    i32.sub
    global.set 2
    i64.const 39584465551547
    i64.popcnt
    ref.null extern
    local.get 1
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.neg
    f64.floor
    local.tee 3
    f64.const nan (;=NaN;)
    local.get 3
    local.get 3
    f64.eq
    select
    i32.trunc_f64_u
    v128.const i32x4 0xffffffff 0xffffffff 0xffffffff 0xffffffff
    f64x2.nearest
    local.tee 4
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 4
    local.get 4
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 5
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 5
    local.get 5
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 6
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 6
    local.get 6
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 7
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 7
    local.get 7
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 8
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 8
    local.get 8
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 9
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 9
    local.get 9
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 10
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 10
    local.get 10
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 11
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 11
    local.get 11
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 12
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 12
    local.get 12
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 13
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 13
    local.get 13
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 14
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 14
    local.get 14
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 15
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 15
    local.get 15
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 16
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 16
    local.get 16
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 17
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 17
    local.get 17
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 18
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 18
    local.get 18
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 19
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 19
    local.get 19
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 20
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 20
    local.get 20
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 21
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 21
    local.get 21
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 22
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 22
    local.get 22
    f64x2.eq
    v128.bitselect
    f64x2.nearest
    local.tee 23
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 23
    local.get 23
    f64x2.eq
    v128.bitselect
    i64.const 720575940379279359
    i32.wrap_i64
    i8x16.shr_s
    f32x4.convert_i32x4_s
    f64x2.floor
    local.tee 24
    v128.const i32x4 0x00000000 0x7ff80000 0x00000000 0x7ff80000
    local.get 24
    local.get 24
    f64x2.eq
    v128.bitselect
    global.get 0
    v128.xor
    global.set 0
    global.get 1
    i32.xor
    global.set 1
    drop
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 0)
  (global (;2;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (global 0))
  (export "2" (global 1))
)
