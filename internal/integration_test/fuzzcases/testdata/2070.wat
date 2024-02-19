(module
  (func (local i64)
    i64.const 0
    i64.const 0
    i64.div_s
    local.set 0
    local.get 0
    loop (param i64) (result i32) ;; label = @1
      local.get 0
      br 0 (;@1;)
    end
    unreachable
  )
)
