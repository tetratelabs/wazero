(module
    (global $n i32 (i32.const -1))
    (func $extend (export "extend") (result i64)
        global.get $n
        i64.extend_i32_u
    )
)
