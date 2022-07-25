(module
  (func (export "select on conditional value after table.size") (result i32)
    i32.const 1234
    ref.null func
    ref.is_null
    ;; At this point, the result of ref.is_null (=1) is on the conditional register.
    ;; If table.size doesn't save the value into a general purpose register,
    ;; the result of select below becomes incorrect.
    table.size 0 ;; -> 0
    select ;; -> select the result of ref.is_null == 1.
  )
  (table 0 0 funcref)
)
