(module
  (func
    v128.const i32x4 0x8000_8000 0 0 0
    i32x4.extend_low_i16x8_s
    f64x2.trunc
    i32x4.extend_low_i16x8_s
    f64x2.extract_lane 0
    i64.trunc_f64_s
    drop
  )
  (start 0)
)
