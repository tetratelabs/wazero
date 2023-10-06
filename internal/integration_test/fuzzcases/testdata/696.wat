(module
  (func $dummy)
  (func (export "select") (param i32) (result v128)
    v128.const i64x2 0xffffffffffffffff 0xeeeeeeeeeeeeeeee
    v128.const i64x2 0x1111111111111111 0x2222222222222222
    local.get 0
    call 0  ;; calling dummy function before select to
    select
  )

  (func (export "typed select") (param i32) (result v128)
    v128.const i64x2 0xffffffffffffffff 0xeeeeeeeeeeeeeeee
    v128.const i64x2 0x1111111111111111 0x2222222222222222
    local.get 0
    call 0  ;; calling dummy function before select to
    select (result v128)
  )
)
