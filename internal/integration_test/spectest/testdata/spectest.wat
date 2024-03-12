(module $spectest
  (global (export "global_i32") i32 (i32.const 666))
  (global (export "global_i64") i64 (i64.const 666))
  (global (export "global_f32") f32 (f32.const 666.6))
  (global (export "global_f64") f64 (f64.const 666.6))

  (table (export "table") 10 20 funcref)

  (memory 1 2)
    (export "memory" (memory 0))

;; Note: the following aren't host functions that print to console as it would clutter it. These only drop the inputs.
  (func)
     (export "print" (func 0))

  (func (param i32) local.get 0 drop)
     (export "print_i32" (func 1))

  (func (param i64) local.get 0 drop)
     (export "print_i64" (func 2))

  (func (param f32) local.get 0 drop)
     (export "print_f32" (func 3))

  (func (param f64) local.get 0 drop)
     (export "print_f64" (func 4))

  (func (param i32 f32) local.get 0 drop local.get 1 drop)
     (export "print_i32_f32" (func 5))

  (func (param f64 f64) local.get 0 drop local.get 1 drop)
     (export "print_f64_f64" (func 6))
)
