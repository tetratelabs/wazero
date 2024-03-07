(module
  (type (;0;) (func))
  (func (;0;) (type 0)
    i32.const 0
    v128.load16x4_u offset=655351 align=1
    global.set 0
  )
  (memory (;0;) 10 10 shared)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  (global (;0;) (mut v128) v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000)
  ;; set the initial data at 655351
  (data (i32.const 655351) "CD")
  (start 0)
)
