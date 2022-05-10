;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/ref_is_null.wast

(module
  (func $f1 (export "funcref") (param $x funcref) (result i32)
    (ref.is_null (local.get $x))
  )
  (func $f2 (export "externref") (param $x externref) (result i32)
    (ref.is_null (local.get $x))
  )

  (table $t1 2 funcref)
  (table $t2 2 externref)
  (elem (table $t1) (i32.const 1) func $dummy)
  (func $dummy)

  (func (export "init") (param $r externref)
    (table.set $t2 (i32.const 1) (local.get $r))
  )
  (func (export "deinit")
    (table.set $t1 (i32.const 1) (ref.null func))
    (table.set $t2 (i32.const 1) (ref.null extern))
  )

  (func (export "funcref-elem") (param $x i32) (result i32)
    (call $f1 (table.get $t1 (local.get $x)))
  )
  (func (export "externref-elem") (param $x i32) (result i32)
    (call $f2 (table.get $t2 (local.get $x)))
  )
)

