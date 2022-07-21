(module
  (func
    i32.const -1
    i32.const 2
    i32.shr_s
    i32.load offset=100
    unreachable
  )
  (memory 1 1)
  (start 0)
)
