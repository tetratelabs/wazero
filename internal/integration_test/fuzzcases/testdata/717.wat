(module
  (func  (export "vectors")
    (result v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 v128 i64 v128)
    v128.const i64x2 0 1
    v128.const i64x2 2 3
    v128.const i64x2 4 5
    v128.const i64x2 6 7
    v128.const i64x2 8 9
    v128.const i64x2 10 11
    v128.const i64x2 12 13
    v128.const i64x2 14 15
    v128.const i64x2 16 17
    v128.const i64x2 18 19
    v128.const i64x2 20 21
    v128.const i64x2 22 23
    v128.const i64x2 24 25
    v128.const i64x2 26 27
    v128.const i64x2 28 29
    v128.const i64x2 30 31
    ;; This makes the following vector(33,34) is not 16-bites aligned in the stack.
    ;; Also, the offset doesn't fit in 9-bit signed integer, therefore the store
    ;; instruction must be correctly encoded as multiple instructions in arm64.
    i64.const 32
    v128.const i64x2 33 34
  )
)
