(module
  ;; Recursive factorial
  (func (export "fac-rec") (param i64) (result i64)
    (if (result i64) (i64.eq (local.get 0) (i64.const 0))
      (then (i64.const 1))
      (else
        (i64.mul (local.get 0) (call 0 (i64.sub (local.get 0) (i64.const 1))))
      )
    )
  )
  ;; Iterative factorial
  (func (export "fac-iter") (param i64) (result i64)
    (local i64 i64)
    (local.set 1 (local.get 0))
    (local.set 2 (i64.const 1))
    (block
      (loop
        (if
          (i64.eq (local.get 1) (i64.const 0))
          (then (br 2))
          (else
            (local.set 2 (i64.mul (local.get 1) (local.get 2)))
            (local.set 1 (i64.sub (local.get 1) (i64.const 1)))
          )
        )
        (br 0)
      )
    )
    (local.get 2)
  )
)
