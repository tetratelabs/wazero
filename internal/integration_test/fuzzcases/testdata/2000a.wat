(module
  (func (param i32) (result i64)
    i32.const 0
    if ;; label = @1
      unreachable
    end
    f32.const -0x1.79f386p+113 (;=-15331525000000000000000000000000000;)
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
    f32.convert_i64_u
    i64.trunc_sat_f32_u
  )
  (export "" (func 0))
)
