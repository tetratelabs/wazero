(module
  (type (func))
  (func (export "i8x16s") (type 0)
    v128.const i8x16 0x0 0xff 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0 0x0
    i8x16.extract_lane_s 1 ;; uint32(int8(0xff)) = 0xffff_ffff
     ;; if the signed extend is 64-bit, then the offset 0xffff_ffff_ffff_ffff + 1= 0 and not result in out of bounds.
    v128.load32_zero offset=1 align=1
    unreachable)
  (func (export "i16x8s") (type 0)
    v128.const i16x8 0x0 0xffff 0x0 0x0 0x0 0x0 0x0 0x0
    i16x8.extract_lane_s 1 ;; uint32(int16(0xffff)) = 0xffff_ffff
     ;; if the signed extend is 64-bit, then the offset 0xffff_ffff_ffff_ffff + 1= 0 and not result in out of bounds.
    v128.load32_zero offset=1 align=1
    unreachable)
  (memory 1 1)
)
