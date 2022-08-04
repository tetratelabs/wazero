(module
  (func (export "out of bounds")
      i32.const 0x100
      ;; The constant offset is calculated as 0xfffffffb + 4 (load target) == 0xffffffff.
      ;; If it is loaded as sign-extended 64-bit int, then the runtime offset calculation results in 0x100 -1 = 255,
      ;; which is *not* ouf-of-bounds access. However, the offset should be 0x1000000ff > 4GBi (maximum possible memory size).
      i32.load offset=0xfffffffb
      drop
  )
  (memory 1 1)
  (export "" (func 0))
)
