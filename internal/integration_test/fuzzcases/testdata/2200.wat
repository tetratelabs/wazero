(module
  (type (;0;) (func (param i32 i32 i32)))
  (type (;1;) (func (param i32 i32 i32) (result i32)))
  (type (;2;) (func (param i32 i32 i32 i32)))
  (type (;3;) (func (param i32 i32)))
  (type (;4;) (func (param i32)))
  (func (;0;) (type 3) (param i32 i32)
    (local i32)
    i32.const 13
    local.set 2
    i32.const 1
    i64.const 1
    i64.store offset=16 align=4
    local.get 2
    local.get 2
    i32.store16 offset=52
    local.get 2
    call 3
    local.get 2
    local.get 2
    i64.load align=4
    i64.store offset=88
    i32.const 0
    local.get 2
    i32.load offset=88
    i32.load offset=88
    unreachable
  )
  (func (;1;) (type 2) (param i32 i32 i32 i32)
    i32.const 0
    i32.const -701700
    i32.store offset=8
    i32.const 1
    i32.const 1
    i32.store offset=8
  )
  (func (;2;) (type 0) (param i32 i32 i32)
    (local i32 i32 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i64 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32 i32)
    v128.const i32x4 0x53525150 0x57565554 0x5b5a5958 0x5f5e5d5c
    local.set 10
    v128.const i32x4 0x00000000 0x00000000 0x00000000 0x00000000
    local.set 20
    i32.const 0
    i32.load offset=8
    local.set 22
    local.get 22
    local.set 1
    i32.const 0
    local.set 28
    local.get 1
    local.tee 30
    local.get 30
    local.get 30
    local.get 30
    call 4
    local.set 31
    i32.const 0
    local.get 19
    v128.store align=4
    i32.const 1
    local.get 18
    v128.store align=4
    i32.const 0
    local.get 17
    v128.store align=4
    local.get 0
    local.get 16
    v128.store align=4
    i32.const 1
    local.get 15
    v128.store align=4
    i32.const 1
    local.get 14
    v128.store align=4
    i32.const 1
    local.get 10
    v128.store align=4
    i32.const 0
    local.get 9
    v128.store align=4
    i32.const 0
    local.get 7
    v128.store align=4
    drop
  )
  (func (;3;) (type 4) (param i32)
    local.get 0
    local.get 0
    local.get 0
    local.get 0
    call 1
    i32.const 1
    i32.const 1
    i32.const 0
    call 2
  )
  (func (;4;) (type 1) (param i32 i32 i32) (result i32)
    i32.const 1
    i32.const 0
    local.get 2
    memory.fill
    i32.const 1
    return
  )
  (memory (;0;) 19)
  (export "" (func 0))
)