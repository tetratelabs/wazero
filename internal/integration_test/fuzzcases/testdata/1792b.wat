(module
  (type (;0;) (func (result f64)))
  (func (;0;) (type 0) (result f64)
    (local i32)
    global.get 1
    i32.eqz
    if  ;; label = @1
      unreachable
    end
    global.get 1
    i32.const 1
    i32.sub
    global.set 1
    ref.null extern
    v128.const i32x4 0xb1b1ffff 0xb1b1b0b1 0xb1b1b1b1 0xffffffb1
    i32x4.bitmask
    global.get 0
    i32.xor
    global.set 0
    drop
    f64.const 0x1.000ffffff0914p+1 (;=2.0004882812219282;)
  )
  (memory (;0;) 10 10)
  (global (;0;) (mut i32) i32.const 0)
  (global (;1;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (export "1" (memory 0))
  (export "2" (global 0))
)