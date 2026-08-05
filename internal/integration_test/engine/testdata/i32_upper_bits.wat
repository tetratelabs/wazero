(module
  (type (;0;) (func (param i32) (result i32)))
  (type (;1;) (func (param i32 i32) (result i32)))
  (func (;0;) (type 0) (param i32) (result i32)
    local.get 0
    i32.eqz)
  (func (;1;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.eq)
  (func (;2;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.ne)
  (func (;3;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.lt_u)
  (func (;4;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.gt_u)
  (func (;5;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.le_u)
  (func (;6;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.ge_u)
  (func (;7;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.lt_s)
  (func (;8;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.gt_s)
  (func (;9;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.le_s)
  (func (;10;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.ge_s)
  (func (;11;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.div_u)
  (func (;12;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.div_s)
  (func (;13;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.rem_u)
  (func (;14;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.rem_s)
  (func (;15;) (type 0) (param i32) (result i32)
    local.get 0
    i32.load)
  (func (;16;) (type 1) (param i32 i32) (result i32)
    local.get 0
    local.get 1
    i32.store
    local.get 0
    i32.load)
  (memory (;0;) 1)
  (export "memory" (memory 0))
  (export "i32_eqz" (func 0))
  (export "i32_eq" (func 1))
  (export "i32_ne" (func 2))
  (export "i32_lt_u" (func 3))
  (export "i32_gt_u" (func 4))
  (export "i32_le_u" (func 5))
  (export "i32_ge_u" (func 6))
  (export "i32_lt_s" (func 7))
  (export "i32_gt_s" (func 8))
  (export "i32_le_s" (func 9))
  (export "i32_ge_s" (func 10))
  (export "i32_div_u" (func 11))
  (export "i32_div_s" (func 12))
  (export "i32_rem_u" (func 13))
  (export "i32_rem_s" (func 14))
  (export "i32_load" (func 15))
  (export "i32_store_load" (func 16)))
