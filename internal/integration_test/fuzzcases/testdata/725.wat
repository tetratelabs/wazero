(module
  (func (export "i32.load8_s")
    i32.const 0 ;; load offset.
    ;; If this sign-extends the data as 64-bit,
    ;; the loaded value becomes -1 in signed-64 bit integer at runtime.
    i32.load8_s offset=0
    ;; Therefore, this load operation access at offset 0 and won't result in out of bounds memory access,
    ;; which is wrong.
    i32.load offset=1
    unreachable
  )
  (func (export "i32.load16_s")
    i32.const 0 ;; load offset.
    ;; If this sign-extends the data as 64-bit,
    ;; the loaded value becomes -1 in signed-64 bit integer at runtime.
    i32.load16_s offset=0
    ;; Therefore, this load operation access at offset 0 and won't result in out of bounds memory access,
    ;; which is wrong.
    i32.load offset=1
    unreachable
  )
  (memory (;0;) 10 10)
  (data (i32.const 0) "\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff")
)
