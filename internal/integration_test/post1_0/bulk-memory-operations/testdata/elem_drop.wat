;; https://github.com/WebAssembly/spec/blob/9b4d86fbcd3309c3365c8430a4ac5ef2126c43a8/test/core/bulk.wast#L244-L259
(module
  (table 1 funcref)
  (func $f)
  (elem $p funcref (ref.func $f))
  (elem $a (table 0) (i32.const 0) func $f)

  (func (export "drop_passive") (elem.drop $p))
  (func (export "init_passive") (param $len i32)
    (table.init $p (i32.const 0) (i32.const 0) (local.get $len))
  )

  (func (export "drop_active") (elem.drop $a))
  (func (export "init_active") (param $len i32)
    (table.init $a (i32.const 0) (i32.const 0) (local.get $len))
  )
)
