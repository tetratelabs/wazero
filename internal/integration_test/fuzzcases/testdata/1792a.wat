(module
  (type (;0;) (func (param f64 f64)))
  (func (;0;) (type 0) (param f64 f64)
    global.get 1
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 1
    i32.const 1
    i32.sub
    global.set 1
    unreachable
    loop  ;; label = @1
      global.get 1
      i32.eqz
      if  ;; label = @2
        unreachable
      end
      global.get 1
      i32.const 1
      i32.sub
      global.set 1
      v128.const i32x4 0x40400200 0xff404040 0x0001ffff 0x00000000
      global.get 0
      v128.xor
      global.set 0
    end
  )
  (table (;0;) 1000 1000 funcref)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;1;) (mut i32) i32.const 1000)
  (export "\00\00\02\00" (func 0))
  (export "" (table 0))
  (export "2" (global 0))
)