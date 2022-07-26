(module
  (func (export "conditional before elem.drop") (result i32)
    ref.null func
    ref.is_null
    ;; At this point, i32 value is placed on the conditional register.
    ;; elem.drop must handle it correctly and save it to a general purpose one.
    elem.drop 0
  )
  (elem func)
)
