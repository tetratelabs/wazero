
(module
  (import "env" "i8x16" (func $i8x16 (param v128) (result v128)))
  (import "env" "i16x8" (func $i16x8 (param v128) (result v128)))
  (import "env" "i32x4" (func $i32x4 (param v128) (result v128)))
  (import "env" "i64x2" (func $i64x2 (param v128) (result v128)))
  (import "env" "f32x4" (func $f32x4 (param v128) (result v128)))
  (import "env" "f64x2" (func $f64x2 (param v128) (result v128)))

  (func (export "call_i8x16") (param v128) (result v128)
    local.get 0
    call $i8x16
  )
  (func (export "call_i16x8") (param v128) (result v128)
    local.get 0
    call $i16x8
  )
  (func (export "call_i32x4") (param v128) (result v128)
    local.get 0
    call $i32x4
  )
  (func (export "call_i64x2") (param v128) (result v128)
    local.get 0
    call $i64x2
  )
  (func (export "call_f32x4") (param v128) (result v128)
    local.get 0
    call $f32x4
  )
  (func (export "call_f64x2") (param v128) (result v128)
    local.get 0
    call $f64x2
  )
)
