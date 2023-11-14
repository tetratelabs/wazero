(module
  (memory (;0;) 1 1)
  (func (;0;)
    i32.const 0
    v128.const i32x4 0x80000000 0x80000000 0x80000000 0x80000000
    v128.store32_lane offset=15616 align=2 1
    i32.const 0
    v128.load32_splat offset=15616 align=2
    global.set 0
  )
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (export "" (func 0))
)
