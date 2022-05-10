;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/table_set.wast#L1-L27
(module
  (table $t2 1 externref)
  (table $t3 2 funcref)
  (elem (table $t3) (i32.const 1) func $dummy)
  (func $dummy)

  (func (export "get-externref") (param $i i32) (result externref)
    (table.get $t2 (local.get $i))
  )
  (func $f3 (export "get-funcref") (param $i i32) (result funcref)
    (table.get $t3 (local.get $i))
  )

  (func (export "set-externref") (param $i i32) (param $r externref)
    (table.set $t2 (local.get $i) (local.get $r))
  )
  (func (export "set-funcref") (param $i i32) (param $r funcref)
    (table.set $t3 (local.get $i) (local.get $r))
  )
  (func (export "set-funcref-from") (param $i i32) (param $j i32)
    (table.set $t3 (local.get $i) (table.get $t3 (local.get $j)))
  )

  (func (export "is_null-funcref") (param $i i32) (result i32)
    (ref.is_null (call $f3 (local.get $i)))
  )
)
