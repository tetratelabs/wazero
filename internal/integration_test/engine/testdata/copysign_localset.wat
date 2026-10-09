(module
  ;; Regression test for #2555: the optimizing compiler miscompiled
  ;; f32.copysign/f64.copysign when the operands were defined by calls whose
  ;; results were moved into locals before the select/copysign sequence.
  (func $g (param f32) (result f32) local.get 0)
  (func (export "select_case") (param $p f32) (result f32)
    local.get $p call $g local.set $p
    local.get $p
    local.get $p call $g
    f32.const 0 i32.const 1 select
    f32.copysign)
  (func (export "negative_sign") (param $p f32) (result f32)
    local.get $p call $g local.set $p
    local.get $p
    f32.const -1.5 call $g
    f32.const -0 i32.const 1 select
    f32.copysign))
