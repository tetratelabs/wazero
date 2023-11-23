(module
  (type (;0;) (func (result f64 f64 f32 f64 f64 f32 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)))
  (type (;1;) (func (result f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)))
  (type (;2;) (func))
  (func (;0;) (type 1) (result f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64)
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0
      f64.const 0.0

;;    unreachable
  )
  (func (;1;) (type 2)
    (local f64 f64 i32)
    i32.const 0
    if (type 0) (result f64 f64 f32 f64 f64 f32 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64 f64) ;; label = @1
      f64.const 1.0
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
      f64.const 0x0p+0 (;=0;)
      f32.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f32.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
    else
      f64.const 2.0
      f64.const 0x0p+0 (;=0;)
      f32.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f32.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const -0x1.a2d89e0481a8dp+83 (;=-15823560583422023000000000;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
      f64.const 0x0p+0 (;=0;)
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
    global.set 0
  )
  (global (;0;) (mut i64) i64.const 0)
  (global (;1;) (mut i32) i32.const 0)
  (export "" (func 1))
)
