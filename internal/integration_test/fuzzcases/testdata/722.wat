(module
  (func (export "conditional before data.drop") (result i32)
    ref.null func
    ref.is_null
    ;; At this point, i32 value is placed on the conditional register.
    ;; data.drop must handle it correctly and save it to a general purpose one.
    data.drop 0
  )
  (data "\ff")
)
