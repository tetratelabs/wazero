;; https://github.com/WebAssembly/spec/blob/9b4d86fbcd3309c3365c8430a4ac5ef2126c43a8/test/core/bulk.wast#L199-L216
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
