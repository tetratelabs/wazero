(module
  (func
    i32.const -1
    i32.const 2
    i32.shr_s
    ;; The shifted i32 should have the higher 32-bit cleared. If the result is signed-extended to 64-bit integer,
    ;; which means it has higher 32-bit all set, this i32.load calculates offset as int64(-1 << 2) + 4 = 0.
    ;; In that case, this won't rseult in memory out of bounds, which is not correct behavior.
    i32.load offset=4
    drop
  )
  (memory 1)
  (start 0)
)
