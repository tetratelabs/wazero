;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/ref_func.wast#L80-L106

(module
  (func $f1)
  (func $f2)
  (func $f3)
  (func $f4)
  (func $f5)
  (func $f6)

  (table $t 1 funcref)

  (global funcref (ref.func $f1))
  (export "f" (func $f2))
  (elem (table $t) (i32.const 0) func $f3)
  (elem (table $t) (i32.const 0) funcref (ref.func $f4))
  (elem func $f5)
  (elem funcref (ref.func $f6))

  (func
    (ref.func $f1)
    (ref.func $f2)
    (ref.func $f3)
    (ref.func $f4)
    (ref.func $f5)
    (ref.func $f6)
    (return)
  )
)
