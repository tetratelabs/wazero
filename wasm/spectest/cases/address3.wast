;; https://github.com/WebAssembly/spec/blob/d404d096bcdbe7f663bd6b2384dd65c8023b8312/test/core/address.wast
;; Load f32 data with different offset/align arguments

(module
  (memory 1)
  (data (i32.const 0) "\00\00\00\00\00\00\a0\7f\01\00\d0\7f")

  (func (export "32_good1") (param $i i32) (result f32)
    (f32.load offset=0 (local.get $i))                   ;; 0.0 '\00\00\00\00'
  )
  (func (export "32_good2") (param $i i32) (result f32)
    (f32.load align=1 (local.get $i))                    ;; 0.0 '\00\00\00\00'
  )
  (func (export "32_good3") (param $i i32) (result f32)
    (f32.load offset=1 align=1 (local.get $i))           ;; 0.0 '\00\00\00\00'
  )
  (func (export "32_good4") (param $i i32) (result f32)
    (f32.load offset=2 align=2 (local.get $i))           ;; 0.0 '\00\00\00\00'
  )
  (func (export "32_good5") (param $i i32) (result f32)
    (f32.load offset=8 align=4 (local.get $i))           ;; nan:0x500001 '\01\00\d0\7f'
  )
  (func (export "32_bad") (param $i i32)
    (drop (f32.load offset=4294967295 (local.get $i)))
  )
)

;; assertions

;; (assert_return (invoke "32_good1" (i32.const 0)) (f32.const 0.0))
;; (assert_return (invoke "32_good2" (i32.const 0)) (f32.const 0.0))
;; (assert_return (invoke "32_good3" (i32.const 0)) (f32.const 0.0))
;; (assert_return (invoke "32_good4" (i32.const 0)) (f32.const 0.0))
;; (assert_return (invoke "32_good5" (i32.const 0)) (f32.const nan:0x500001))
;; (assert_return (invoke "32_good1" (i32.const 65524)) (f32.const 0.0))
;; (assert_return (invoke "32_good2" (i32.const 65524)) (f32.const 0.0))
;; (assert_return (invoke "32_good3" (i32.const 65524)) (f32.const 0.0))
;; (assert_return (invoke "32_good4" (i32.const 65524)) (f32.const 0.0))
;; (assert_return (invoke "32_good5" (i32.const 65524)) (f32.const 0.0))
;; (assert_return (invoke "32_good1" (i32.const 65525)) (f32.const 0.0))
;; (assert_return (invoke "32_good2" (i32.const 65525)) (f32.const 0.0))
;; (assert_return (invoke "32_good3" (i32.const 65525)) (f32.const 0.0))
;; (assert_return (invoke "32_good4" (i32.const 65525)) (f32.const 0.0))
;; (assert_trap (invoke "32_good5" (i32.const 65525)) "out of bounds memory access")
;; (assert_trap (invoke "32_good3" (i32.const -1)) "out of bounds memory access")
;; (assert_trap (invoke "32_good3" (i32.const -1)) "out of bounds memory access")
;; (assert_trap (invoke "32_bad" (i32.const 0)) "out of bounds memory access")
;; (assert_trap (invoke "32_bad" (i32.const 1)) "out of bounds memory access")
