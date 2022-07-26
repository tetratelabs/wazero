(module
  (func (export "require unreachable") (local v128) ;; having v128 local results in the non-trivial register allocation in the v128.const below.
    v128.const i64x2 0x1 0x2
    i64x2.all_true ;; must be non zero since i64x2(0x1, 0x2) is non zero on all lanes.
    i32.eqz ;; must be 0 as the result ^ is not zero.
    br_if 0 ;; return target, but the rseult of i32.eqz is zero, therefore branching shoulnd't happen.
    unreachable ;; Hence, we reach this unreachable instruction.
  )
)
