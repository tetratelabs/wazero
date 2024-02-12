(module
  (type (;0;) (func (param i32)))
  (func (;0;) (type 0) (param i32)
    i32.const 988345840
    f32.convert_i32_s
    global.get 0 ;;f32.const 0x1.c8c8c8p+73
    f32.max
    f32.const nan (;=nan;)
    i32.const 1
    select
    i32.reinterpret_f32
    i32.const 0 ;; global.get 1
    i32.xor
    global.set 2) ;; global.set 1 makes no difference
  (global (;0;) f32 (f32.const 0x1.c8c8c8p+73 (;=1.68524e+22;)))
  (global (;1;) (mut i32) (i32.const 0))
  (global (;2;) (mut i32) (i32.const 1000))
  (export "" (func 0))
  (export "1" (global 0))
  (export "2" (global 1))
  (data (;0;) ""))
