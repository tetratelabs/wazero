(module
  (func (export "i32.extend16_s")
    i32.const 0xffff
    ;; if this extends to 64 bit, the bit pattern of the value has all bits set
    i32.extend16_s
    ;; then plus one to it results in zero offset.
    v128.load16x4_u offset=1 align=1
    unreachable
  )
  (func (export "i32.extend8_s")
    i32.const 0xff
    ;; if this extends to 64 bit, the bit pattern of the value has all bits set
    i32.extend8_s
    ;; then plus one to it results in zero offset.
    v128.load16x4_u offset=1 align=1
    unreachable
  )
  (memory 1 1)
)
