(module
  (func
    i32.const -1
    v128.const i32x4 0x7e7e7e7e 0x00000006 0x00000000 0x7e7e7e7e
    v128.store align=1
  )
  (memory (;0;) 7 10)
  (export "" (func 0))
)
