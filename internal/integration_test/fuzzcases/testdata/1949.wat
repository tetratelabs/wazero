(module
   (func
    i32.const 0
    i32.const 0xcafecafe
    i32.store offset=65526 align=1

    i32.const 0
    v128.const i32x4 0xc1c1c1c1 0xcacac1c1 0xcacacaca 0xcacacaca
    v128.store offset=65526 align=1
  )
  (memory (;0;) 1 7)
  (export "" (func 0))
)
