(module $counter
  ;; get returns the next counter value
  (func (export "get") (import "env" "next_i32") (result i32))
)
