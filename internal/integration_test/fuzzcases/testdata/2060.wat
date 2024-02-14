(module
  (type (;0;) (func (param i32)))
  (func (;0;) (type 0) (param i32)
    (local v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128)
    i32.const 0
    if ;; label = @1
      unreachable
    end
    local.get 13
    local.get 13
    f64x2.eq
    global.set 1
    global.get 1
    i64x2.abs
    global.set 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 53)
  (export "" (func 0))
)
