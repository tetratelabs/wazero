;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/ref_func.wast#L68-L74
(module
  (func $f (import "M" "f") (param i32) (result i32))
  (func $g (import "M" "g") (param i32) (result i32))
  (global funcref (ref.func 7))
)
