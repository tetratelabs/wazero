(module
  (func (export "out of bounds")
      i32.const 0x100
      ;; The constant offset is calculated as 0xfffffffb + 4 (load target) == 0xffffffff.
      ;; If it is loaded as sign-extended 64-bit int, then the runtime offset calculation results in 0x100 -1 = 255,
      ;; which is *not* ouf-of-bounds access. However, the offset should be 0x1000000ff > 65536.
      i32.load offset=0xfffffffb
      drop
  )
  (func (export "store higher offset")
    i32.const 32769
    memory.grow
    drop
    i32.const 0x100 ;; runtime offset
    i64.const 0xffffffffffffffff ;; store target value.
    ;; This stores at 0x80000100 which lies in the last page, and 0x80000100 is
    ;; larger than math.MaxInt32, therefore in amd64 the offset calculation becomes two instructions.
    i64.store offset=0x80000000
  )
  (memory 1 32770) ;; allows 65536*32770 = 0x80020000 bytes.
  (export "" (func 0))
)
