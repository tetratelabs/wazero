;; Extended contant expressions

(module
  (memory 1)
  (data (i32.add (i32.const 0) (i32.const 42)))
)

(module
  (memory 1)
  (data (i32.sub (i32.const 42) (i32.const 0)))
)

(module
  (memory 1)
  (data (i32.mul (i32.const 1) (i32.const 2)))
)

;; Combining add, sub, mul and global.get

(module
  (global (import "spectest" "global_i32") i32)
  (memory 1)
  (data (i32.mul
          (i32.const 2)
          (i32.add
            (i32.sub (global.get 0) (i32.const 1))
            (i32.const 2)
          )
        )
  )
)
