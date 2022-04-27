;; This file includes changes to test/core/call_indirect.wast from the commit that added "multi-value" support.
;;
;; Compile like so, in order to not add any other post 1.0 features to the resulting wasm.
;;  wat2wasm \
;;      --disable-saturating-float-to-int \
;;      --disable-sign-extension \
;;      --disable-simd \
;;      --disable-bulk-memory  \
;;      --disable-reference-types  \
;;      call_indirect.wat
;;
;; See https://github.com/WebAssembly/spec/commit/484180ba3d9d7638ba1cb400b699ffede796927c
(module $call_indirect.wast

;; preconditions
  (type $proc (func))
  (type $out-i32 (func (result i32)))
  (type $out-i64 (func (result i64)))
  (type $out-f32 (func (result f32)))
  (type $out-f64 (func (result f64)))
  (type $f32-i32 (func (param f32 i32) (result i32)))
  (type $i32-i64 (func (param i32 i64) (result i64)))
  (type $f64-f32 (func (param f64 f32) (result f32)))
  (type $i64-f64 (func (param i64 f64) (result f64)))
  (type $over-i32 (func (param i32) (result i32)))
  (type $over-i64 (func (param i64) (result i64)))
  (type $over-f32 (func (param f32) (result f32)))
  (type $over-f64 (func (param f64) (result f64)))
  (type $over-i32-duplicate (func (param i32) (result i32)))
  (type $over-i64-duplicate (func (param i64) (result i64)))
  (type $over-f32-duplicate (func (param f32) (result f32)))
  (type $over-f64-duplicate (func (param f64) (result f64)))

  (func $const-i32 (type $out-i32) (i32.const 0x132))
  (func $const-i64 (type $out-i64) (i64.const 0x164))
  (func $const-f32 (type $out-f32) (f32.const 0xf32))
  (func $const-f64 (type $out-f64) (f64.const 0xf64))
  (func $id-i32 (type $over-i32) (local.get 0))
  (func $id-i64 (type $over-i64) (local.get 0))
  (func $id-f32 (type $over-f32) (local.get 0))
  (func $id-f64 (type $over-f64) (local.get 0))
  (func $i32-i64 (type $i32-i64) (local.get 1))
  (func $i64-f64 (type $i64-f64) (local.get 1))
  (func $f32-i32 (type $f32-i32) (local.get 1))
  (func $f64-f32 (type $f64-f32) (local.get 1))
  (func $over-i32-duplicate (type $over-i32-duplicate) (local.get 0))
  (func $over-i64-duplicate (type $over-i64-duplicate) (local.get 0))
  (func $over-f32-duplicate (type $over-f32-duplicate) (local.get 0))
  (func $over-f64-duplicate (type $over-f64-duplicate) (local.get 0))
  (func (export "dispatch") (param i32 i64) (result i64)
    (call_indirect (type $over-i64) (local.get 1) (local.get 0))
  )
  (func $fac-i64 (export "fac-i64") (type $over-i64)
    (if (result i64) (i64.eqz (local.get 0))
      (then (i64.const 1))
      (else
        (i64.mul
          (local.get 0)
          (call_indirect (type $over-i64)
            (i64.sub (local.get 0) (i64.const 1))
            (i32.const 12)
          )
        )
      )
    )
  )

  (func $fib-i64 (export "fib-i64") (type $over-i64)
    (if (result i64) (i64.le_u (local.get 0) (i64.const 1))
      (then (i64.const 1))
      (else
        (i64.add
          (call_indirect (type $over-i64)
            (i64.sub (local.get 0) (i64.const 2))
            (i32.const 13)
          )
          (call_indirect (type $over-i64)
            (i64.sub (local.get 0) (i64.const 1))
            (i32.const 13)
          )
        )
      )
    )
  )

  (func $fac-i32 (export "fac-i32") (type $over-i32)
    (if (result i32) (i32.eqz (local.get 0))
      (then (i32.const 1))
      (else
        (i32.mul
          (local.get 0)
          (call_indirect (type $over-i32)
            (i32.sub (local.get 0) (i32.const 1))
            (i32.const 23)
          )
        )
      )
    )
  )

  (func $fac-f32 (export "fac-f32") (type $over-f32)
    (if (result f32) (f32.eq (local.get 0) (f32.const 0.0))
      (then (f32.const 1.0))
      (else
        (f32.mul
          (local.get 0)
          (call_indirect (type $over-f32)
            (f32.sub (local.get 0) (f32.const 1.0))
            (i32.const 24)
          )
        )
      )
    )
  )

  (func $fac-f64 (export "fac-f64") (type $over-f64)
    (if (result f64) (f64.eq (local.get 0) (f64.const 0.0))
      (then (f64.const 1.0))
      (else
        (f64.mul
          (local.get 0)
          (call_indirect (type $over-f64)
            (f64.sub (local.get 0) (f64.const 1.0))
            (i32.const 25)
          )
        )
      )
    )
  )

  (func $fib-i32 (export "fib-i32") (type $over-i32)
    (if (result i32) (i32.le_u (local.get 0) (i32.const 1))
      (then (i32.const 1))
      (else
        (i32.add
          (call_indirect (type $over-i32)
            (i32.sub (local.get 0) (i32.const 2))
            (i32.const 26)
          )
          (call_indirect (type $over-i32)
            (i32.sub (local.get 0) (i32.const 1))
            (i32.const 26)
          )
        )
      )
    )
  )

  (func $fib-f32 (export "fib-f32") (type $over-f32)
    (if (result f32) (f32.le (local.get 0) (f32.const 1.0))
      (then (f32.const 1.0))
      (else
        (f32.add
          (call_indirect (type $over-f32)
            (f32.sub (local.get 0) (f32.const 2.0))
            (i32.const 27)
          )
          (call_indirect (type $over-f32)
            (f32.sub (local.get 0) (f32.const 1.0))
            (i32.const 27)
          )
        )
      )
    )
  )

  (func $fib-f64 (export "fib-f64") (type $over-f64)
    (if (result f64) (f64.le (local.get 0) (f64.const 1.0))
      (then (f64.const 1.0))
      (else
        (f64.add
          (call_indirect (type $over-f64)
            (f64.sub (local.get 0) (f64.const 2.0))
            (i32.const 28)
          )
          (call_indirect (type $over-f64)
            (f64.sub (local.get 0) (f64.const 1.0))
            (i32.const 28)
          )
        )
      )
    )
  )

  (func $even (export "even") (param i32) (result i32)
    (if (result i32) (i32.eqz (local.get 0))
      (then (i32.const 44))
      (else
        (call_indirect (type $over-i32)
          (i32.sub (local.get 0) (i32.const 1))
          (i32.const 15)
        )
      )
    )
  )

  (func $odd (export "odd") (param i32) (result i32)
    (if (result i32) (i32.eqz (local.get 0))
      (then (i32.const 99))
      (else
        (call_indirect (type $over-i32)
          (i32.sub (local.get 0) (i32.const 1))
          (i32.const 14)
        )
      )
    )
  )

  (func $runaway (export "runaway") (call_indirect (type $proc) (i32.const 16)))

  (func $mutual-runaway1 (export "mutual-runaway") (call_indirect (type $proc) (i32.const 18)))

  (func $mutual-runaway2 (call_indirect (type $proc) (i32.const 17)))

;; changes
  (type $out-f64-i32 (func (result f64 i32)))
  (type $over-i32-f64 (func (param i32 f64) (result i32 f64)))
  (type $swap-i32-i64 (func (param i32 i64) (result i64 i32)))

  (table funcref
    (elem
      $const-i32 $const-i64 $const-f32 $const-f64  ;; 0..3
      $id-i32 $id-i64 $id-f32 $id-f64              ;; 4..7
      $f32-i32 $i32-i64 $f64-f32 $i64-f64          ;; 9..11
      $fac-i64 $fib-i64 $even $odd                 ;; 12..15
      $runaway $mutual-runaway1 $mutual-runaway2   ;; 16..18
      $over-i32-duplicate $over-i64-duplicate      ;; 19..20
      $over-f32-duplicate $over-f64-duplicate      ;; 21..22
      $fac-i32 $fac-f32 $fac-f64                   ;; 23..25
      $fib-i32 $fib-f32 $fib-f64                   ;; 26..28
      $const-f64-i32 $id-i32-f64 $swap-i32-i64     ;; 29..31
    )
  )

  (func $const-f64-i32 (type $out-f64-i32) (f64.const 0xf64) (i32.const 32))

  (func $id-i32-f64 (type $over-i32-f64) (local.get 0) (local.get 1))

  (func $swap-i32-i64 (type $swap-i32-i64) (local.get 1) (local.get 0))

  (func (export "type-f64-i32") (result f64 i32)
    (call_indirect (type $out-f64-i32) (i32.const 29))
  )

  (func (export "type-all-f64-i32") (result f64 i32)
    (call_indirect (type $out-f64-i32) (i32.const 29))
  )

  (func (export "type-all-i32-f64") (result i32 f64)
    (call_indirect (type $over-i32-f64)
      (i32.const 1) (f64.const 2) (i32.const 30)
    )
  )

  (func (export "type-all-i32-i64") (result i64 i32)
    (call_indirect (type $swap-i32-i64)
      (i32.const 1) (i64.const 2) (i32.const 31)
    )
  )
)
