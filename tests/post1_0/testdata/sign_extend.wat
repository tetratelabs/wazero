(module
  (func (export "i32.extend8_s") (param $x i32) (result i32) (i32.extend8_s (local.get $x)))
  (func (export "i32.extend16_s") (param $x i32) (result i32) (i32.extend16_s (local.get $x)))
  (func (export "i64.extend8_s") (param $x i64) (result i64) (i64.extend8_s (local.get $x)))
  (func (export "i64.extend16_s") (param $x i64) (result i64) (i64.extend16_s (local.get $x)))
  (func (export "i64.extend32_s") (param $x i64) (result i64) (i64.extend32_s (local.get $x)))
)
