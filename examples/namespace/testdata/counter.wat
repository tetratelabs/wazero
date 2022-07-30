(module $counter
  ;; import `next_i32` function from the `env` module.
  (import "env" "next_i32" (func (result i32)))
  ;; get returns the next counter value by calling the imported `next_i32` function.
  (func (export "get") (result i32) (call 0))
)
