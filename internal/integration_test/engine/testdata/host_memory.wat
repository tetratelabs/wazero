(module $test
  (import "host" "store_int"
    (func $store_int (param $offset i32) (param $val i64) (result (;errno;) i32)))
  (memory $memory 1 1)
  (export "memory" (memory $memory))
  (func (param i32) (param i64) (result i32)
    local.get 0
    local.get 1
    call 0
  )
  ;; store_int is imported from the environment.
  (export "store_int" (func 1))
)
