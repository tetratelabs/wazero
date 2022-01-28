(module
    (memory 0)
    (func $grow (export "grow") (param $sz i32) (result i32) (memory.grow (local.get $sz)))
    (func $size (export "size") (result i32) (memory.size))
)
