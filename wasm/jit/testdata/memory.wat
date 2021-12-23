(module
    (memory 0)
    (func (export "grow") (param $sz i32) (result i32) (memory.grow (local.get $sz)))
    (func (export "size") (result i32) (memory.size))
)
