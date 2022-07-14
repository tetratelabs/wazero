(module
  (func $dummy)
  (func (export "select with 0 / after calling dummy")
    v128.const i64x2 0xffffffffffffffff 0xffffffffffffffff
    v128.const i64x2 0xeeeeeeeeeeeeeeee 0xeeeeeeeeeeeeeeee
    i32.const 0 ;; choose 0xeeeeeeeeeeeeeeee lane.
    call 0  ;; calling dummy function before select to
    select
    ;; check the equality.
    i64x2.extract_lane 0
    i64.const 0xeeeeeeeeeeeeeeee
    i64.eq
    (if
      (then)
      (else unreachable)
    )
  )

  (func (export "select with 0")
    v128.const i64x2 0xffffffffffffffff 0xffffffffffffffff
    v128.const i64x2 0xeeeeeeeeeeeeeeee 0xeeeeeeeeeeeeeeee
    i32.const 0 ;; choose 0xeeeeeeeeeeeeeeee lane.
    select
    ;; check the equality.
    i64x2.extract_lane 0
    i64.const 0xeeeeeeeeeeeeeeee
    i64.eq
    (if
      (then)
      (else unreachable)
    )
  )

  (func (export "typed select with 1 / after calling dummy")
    v128.const i64x2 0xffffffffffffffff 0xffffffffffffffff
    v128.const i64x2 0xeeeeeeeeeeeeeeee 0xeeeeeeeeeeeeeeee
    i32.const 1 ;; choose 0xffffffffffffffff lane.
    call 0  ;; calling dummy function before select to
    select (result v128)
    ;; check the equality.
    i64x2.extract_lane 0
    i64.const 0xffffffffffffffff
    i64.eq
    (if
      (then)
      (else unreachable)
    )
  )

  (func (export "typed select with 1")
    v128.const i64x2 0xffffffffffffffff 0xffffffffffffffff
    v128.const i64x2 0xeeeeeeeeeeeeeeee 0xeeeeeeeeeeeeeeee
    i32.const 1 ;; choose 0xffffffffffffffff lane.
    select (result v128)
    ;; check the equality.
    i64x2.extract_lane 0
    i64.const 0xffffffffffffffff
    i64.eq
    (if
      (then)
      (else unreachable)
    )
  )
)
