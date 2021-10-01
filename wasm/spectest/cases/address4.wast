;; https://github.com/WebAssembly/spec/blob/d404d096bcdbe7f663bd6b2384dd65c8023b8312/test/core/address.wast
;; Load f64 data with different offset/align arguments

(module
  (memory 1)
  (data (i32.const 0) "\00\00\00\00\00\00\00\00\00\00\00\00\00\00\00\00\f4\7f\01\00\00\00\00\00\fc\7f")

  (func (export "64_good1") (param $i i32) (result f64)
    (f64.load offset=0 (local.get $i))                     ;; 0.0 '\00\00\00\00\00\00\00\00'
  )
  (func (export "64_good2") (param $i i32) (result f64)
    (f64.load align=1 (local.get $i))                      ;; 0.0 '\00\00\00\00\00\00\00\00'
  )
  (func (export "64_good3") (param $i i32) (result f64)
    (f64.load offset=1 align=1 (local.get $i))             ;; 0.0 '\00\00\00\00\00\00\00\00'
  )
  (func (export "64_good4") (param $i i32) (result f64)
    (f64.load offset=2 align=2 (local.get $i))             ;; 0.0 '\00\00\00\00\00\00\00\00'
  )
  (func (export "64_good5") (param $i i32) (result f64)
    (f64.load offset=18 align=8 (local.get $i))            ;; nan:0xc000000000001 '\01\00\00\00\00\00\fc\7f'
  )
  (func (export "64_bad") (param $i i32)
    (drop (f64.load offset=4294967295 (local.get $i)))
  )
)

;; (assert_return (invoke "64_good1" (i32.const 0)) (f64.const 0.0))
;; (assert_return (invoke "64_good2" (i32.const 0)) (f64.const 0.0))
;; (assert_return (invoke "64_good3" (i32.const 0)) (f64.const 0.0))
;; (assert_return (invoke "64_good4" (i32.const 0)) (f64.const 0.0))
;; (assert_return (invoke "64_good5" (i32.const 0)) (f64.const nan:0xc000000000001))
;; (assert_return (invoke "64_good1" (i32.const 65510)) (f64.const 0.0))
;; (assert_return (invoke "64_good2" (i32.const 65510)) (f64.const 0.0))
;; (assert_return (invoke "64_good3" (i32.const 65510)) (f64.const 0.0))
;; (assert_return (invoke "64_good4" (i32.const 65510)) (f64.const 0.0))
;; (assert_return (invoke "64_good5" (i32.const 65510)) (f64.const 0.0))
;; (assert_return (invoke "64_good1" (i32.const 65511)) (f64.const 0.0))
;; (assert_return (invoke "64_good2" (i32.const 65511)) (f64.const 0.0))
;; (assert_return (invoke "64_good3" (i32.const 65511)) (f64.const 0.0))
;; (assert_return (invoke "64_good4" (i32.const 65511)) (f64.const 0.0))
;; (assert_trap (invoke "64_good5" (i32.const 65511)) "out of bounds memory access")
;; (assert_trap (invoke "64_good3" (i32.const -1)) "out of bounds memory access")
;; (assert_trap (invoke "64_good3" (i32.const -1)) "out of bounds memory access")
;; (assert_trap (invoke "64_bad" (i32.const 0)) "out of bounds memory access")
;; (assert_trap (invoke "64_bad" (i32.const 1)) "out of bounds memory access")
