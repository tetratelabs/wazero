(module
  ;; env.f must be a host function for benchmarks on the cost of host calls which cross the Wasm<>Go boundary.
  (func $host_func (import "env" "f") (param i64) (result i64))
  ;; call_host_func calls "env.f" and returns the resut as-is.
  (func (export "call_host_func") (param i64) (result i64)
    local.get 0
    call $host_func
  )
)
