(module
  (type (;0;) (func (param funcref funcref f32)))
  (func (;0;) (type 0) (param funcref funcref f32)
    (local v128)
    global.get 1
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 1
    i32.const 1
    i32.sub
    global.set 1
    v128.const i32x4 0x0145ff40 0x45ffffff 0xffff4545 0xffffffff
    f32x4.trunc
    local.tee 3
    v128.const i32x4 0x7fc00000 0x7fc00000 0x7fc00000 0x7fc00000
    local.get 3
    local.get 3
    f32x4.eq
    v128.bitselect
    global.get 0
    v128.xor
    global.set 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (global 0))
)