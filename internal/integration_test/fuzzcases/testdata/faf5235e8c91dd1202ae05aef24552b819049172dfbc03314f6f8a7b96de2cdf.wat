(module
  (type (;0;) (func (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)))
  (type (;1;) (func (result f64 f64 f32 f64 f64 f32 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)))
  (type (;2;) (func (param i32)))
  (func (;0;) (result f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
    f64.const 0
  )
  (func (;1;)
    (local f64 f64 i32)
    i32.const 0
    if (result f64 f64 f32 f64 f64 f32 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)  ;; label = @1
      f64.const 1
      call 0

      drop
      drop
      drop
      drop
      drop
      drop
      drop
      drop
      drop
      drop
      drop
      drop

      f64.const 0
      f32.const 0
      f64.const 0
      f64.const 0
      f32.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
    else
      f64.const 2
      f64.const 0
      f32.const 0
      f64.const 0
      f64.const 0
      f32.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
      f64.const 0
    end
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop
    drop

    i64.reinterpret_f64
    global.set 0 ;; last failure
  )
  (table (;0;) 1000 1000 externref)
  (table (;1;) 827 828 funcref)
  (memory (;0;) 0 3)
  (global (;0;) (mut i64) i64.const 0)
  (global (;1;) (mut i32) i32.const 0)
  (global (;2;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;3;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "9" (func 1))
  (export "12" (table 0))
  (export "13" (table 1))
  (export "14" (memory 0))
  (export "15" (global 0))
  (export "16" (global 1))
  (export "17" (global 2))
  (elem (;0;) (i32.const 0) externref)
  (data (;0;) "\ff")
)
