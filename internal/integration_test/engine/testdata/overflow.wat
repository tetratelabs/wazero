(module
    (global $i32g i32 (i32.const -2147483648)) ;; min of 32-bit signed integer
    (global $i64g i64 (i64.const -9223372036854775808)) ;; min of 64-bit signed integer
    (func $i32 (export "i32") (result i32)
        i32.const 2147483647 ;; max of 32-bit signed integer
        i32.const 1
        i32.add
        global.get $i32g
        i32.eq
    )
    (func $i64 (export "i64") (result i32)
        i64.const 9223372036854775807  ;; max of 64-bit signed integer
        i64.const 1
        i64.add
        global.get $i64g
        i64.eq
    )
)
