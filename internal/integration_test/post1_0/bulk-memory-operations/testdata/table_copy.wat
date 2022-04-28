;; https://github.com/WebAssembly/spec/blob/9b4d86fbcd3309c3365c8430a4ac5ef2126c43a8/test/core/bulk.wast#L300-L316
(module
  (table 10 funcref)
  (elem (i32.const 0) $zero $one $two)
  (func $zero (result i32) (i32.const 0))
  (func $one (result i32) (i32.const 1))
  (func $two (result i32) (i32.const 2))

  (func (export "copy") (param i32 i32 i32)
    (table.copy
      (local.get 0)
      (local.get 1)
      (local.get 2)))

  (func (export "call") (param i32) (result i32)
    (call_indirect (result i32)
      (local.get 0)))
)
