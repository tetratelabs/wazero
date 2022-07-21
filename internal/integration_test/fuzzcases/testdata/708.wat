(module
  (func
    i32.const -1
    i32.const 2
    i32.shr_s
    ;; The shifted i32 should have the higher 32-bit cleared.
    ;; If the result is signed-extended to 64-bit integer,
    ;; then this i32.load calculates offset as int64(-1 << 2) + 4 = 0.
    ;; Therefore, this doesn't rseult in memory out of bounds.
    i32.load offset=4
    drop
  )
  (memory 1)
  (start 0)
)
