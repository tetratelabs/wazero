;; https://github.com/WebAssembly/spec/blob/75059dcb8f3d3e5e8d40f4abd1999f73dfb14b70/test/core/table_grow.wast
(module
  (table $t 0 externref)

  (func (export "get") (param $i i32) (result externref) (table.get $t (local.get $i)))
  (func (export "set") (param $i i32) (param $r externref) (table.set $t (local.get $i) (local.get $r)))

  (func (export "grow") (param $sz i32) (param $init externref) (result i32)
    (table.grow $t (local.get $init) (local.get $sz))
  )
  (func (export "size") (result i32) (table.size $t))
)
