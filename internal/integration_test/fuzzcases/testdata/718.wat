(module
  (func
    i32.const 0
    ;; The ceil of the load operation equals 65528 + 8(=this loads 64-bit = 8 bytes).
    ;; Therefore, this shouldn't result in out of bounds.
    v128.load64_zero offset=65528

    i32.const 0
    ;; The ceil of the load operation equals 65532 + 4(=this loads 32-bit = 4 bytes).
    ;; Therefore, this shouldn't result in out of bounds.
    v128.load32_zero offset=65532

    ;; Drop the loaded values as they are unneede for tests.
    drop
    drop
  )
  (memory 1 1)
  (export "v128.load_zero on the ceil" (func 0))
)
