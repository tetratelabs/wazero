(module
 (type (;0;) (func (param i32 f64)))
 (func (;0;) (type 0) (param i32 f64)
   (local i32 f32 f32)
   i32.const 0
   if ;; label = @1
     unreachable
   end
   f32.const 0x0p+0 (;=0;)
   memory.size
   f32.load offset=93531 align=2
   f32.max
   i32.reinterpret_f32
   i32.const 0
   i32.xor
   global.set 0
 )
 (memory (;0;) 6 8)
 (global (;0;) (mut i32) i32.const 0)
 (export "" (func 0)))
