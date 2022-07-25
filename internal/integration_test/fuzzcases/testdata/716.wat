(module
  (func (result i32)
    i32.const 123
    ref.null func
    ref.is_null ;; -> 1
    ;; At this point, the result of ref.is_null (=1) is on the conditional register.
    ;; If table.size doesn't save the value into a general purpose register,
    ;; the result of select below becomes incorrect.
    ref.func 0
    ref.is_null ;; -> 0
    select ;; should select 1
  )
  (export "select on ref.func" (func 0))
)
