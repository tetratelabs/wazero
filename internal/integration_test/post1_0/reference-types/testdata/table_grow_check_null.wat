;; https://github.com/WebAssembly/spec/blob/75059dcb8f3d3e5e8d40f4abd1999f73dfb14b70/test/core/table_grow.wast#L81-L102
(module
  (table $t 10 funcref)
  (func (export "grow") (param i32) (result i32)
    (table.grow $t (ref.null func) (local.get 0))
  )
  (elem declare func 1)
  (func (export "check-table-null") (param i32 i32) (result funcref)
    (local funcref)
    (local.set 2 (ref.func 1))
    (block
      (loop
        (local.set 2 (table.get $t (local.get 0)))
        (br_if 1 (i32.eqz (ref.is_null (local.get 2))))
        (br_if 1 (i32.ge_u (local.get 0) (local.get 1)))
        (local.set 0 (i32.add (local.get 0) (i32.const 1)))
        (br_if 0 (i32.le_u (local.get 0) (local.get 1)))
      )
    )
    (local.get 2)
  )
)
