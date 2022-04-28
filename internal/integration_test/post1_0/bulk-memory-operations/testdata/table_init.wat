(module
  (table 3 funcref)
  (elem funcref
    (ref.func $zero) (ref.func $one) (ref.func $zero) (ref.func $one))

  (func $zero (result i32) (i32.const 0))
  (func $one (result i32) (i32.const 1))

  (func (export "init") (param i32 i32 i32)
    (table.init 0
      (local.get 0)
      (local.get 1)
      (local.get 2)))

  (func (export "call") (param i32) (result i32)
    (call_indirect (result i32)
      (local.get 0)))
)
