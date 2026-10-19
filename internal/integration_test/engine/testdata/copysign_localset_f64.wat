(module
  (func $g (param f64) (result f64) local.get 0)
  (func (export "select_case") (param $p f64) (result f64)
    local.get $p call $g local.set $p
    local.get $p
    local.get $p call $g
    f64.const 0 i32.const 1 select
    f64.copysign)
  (func (export "negative_sign") (param $p f64) (result f64)
    local.get $p call $g local.set $p
    local.get $p
    f64.const -2.5 call $g
    f64.const -0 i32.const 1 select
    f64.copysign))
