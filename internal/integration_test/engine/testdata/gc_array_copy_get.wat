;; Minimal repro for array.copy + array.get_u on packed i16 arrays.
;; Mirrors the pattern in Binaryen-optimized Kotlin/Wasm output
;; where a string's backing i16 array is built via array.copy then
;; accessed via array.get_u.
(module
  (type $chars (array (mut i16)))

  (func (export "copy_and_get") (result i32)
    (local $src (ref $chars))
    (local $dst (ref $chars))
    ;; Create source array [65, 66, 67] ('A', 'B', 'C')
    (local.set $src (array.new_fixed $chars 3
      (i32.const 65) (i32.const 66) (i32.const 67)))
    ;; Create default destination array of length 3
    (local.set $dst (array.new_default $chars (i32.const 3)))
    ;; Copy all 3 elements: dst[0..3] = src[0..3]
    (array.copy $chars $chars
      (local.get $dst) (i32.const 0)
      (local.get $src) (i32.const 0)
      (i32.const 3))
    ;; Read dst[1] — should return 66 ('B')
    (array.get_u $chars (local.get $dst) (i32.const 1))
  )
)
