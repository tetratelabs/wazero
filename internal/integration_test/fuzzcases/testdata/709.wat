(module
  (func (result v128)
    v128.const i32x4 0xffffffff 0xffffffff 0 0
    ;; This should promote two 32-bit floats on the lower 64-bits (0xffffffff x2)
    ;; Therefore, the returned vector must have non zero lower and higher 64-bits.
    f64x2.promote_low_f32x4
  )
  (export "f64x2.promote_low_f32x4" (func 0))
)
