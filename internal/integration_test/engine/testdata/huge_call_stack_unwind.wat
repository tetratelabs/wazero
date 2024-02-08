(module
  (func
    ;; check global and if it is not zero, call the function again
    (if (i32.ne (global.get $g) (i32.const 0))
      (then
        (global.set $g (i32.sub (global.get $g) (i32.const 1)))
        (call 0)
      )
    )
    ;; otherwise do i32.div by zero and crash
    (i32.div_s (i32.const 0) (i32.const 0))
    drop
  )
  (global $g (mut i32) (i32.const 1000))
  (start 0)
)
