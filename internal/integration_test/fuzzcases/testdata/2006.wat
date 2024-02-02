(module
  (func (param f64)
    i64.const -1891816918
    f64.convert_i64_u
    global.set 1
    global.get 1
    i64.trunc_f64_u
    global.set 2
    global.get 2
    f64.convert_i64_u
    global.set 3
    global.get 3
    i64.trunc_f64_u
    global.set 4
  )
  (export "" (func 0))
  (global (;0;) (mut i64) i64.const 0) ;; dummy
  (global (mut f64) f64.const 0)
  (global (mut i64) i64.const 0)
  (global (mut f64) f64.const 0)
  (global (mut i64) i64.const 0)
)
