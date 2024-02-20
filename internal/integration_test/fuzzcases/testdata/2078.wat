(module
  (type (;0;) (func (param i64 i64)))
  (func (;0;) (type 0) (param i64 i64)
    i32.const 0
    if ;; label = @1
      unreachable
    end
    i32.const 1
    br_if 0 (;@0;)
  )
  (export "" (func 0))
)
