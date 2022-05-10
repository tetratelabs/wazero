;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/ref_null.wast
(module
  (func (export "externref") (result externref) (ref.null extern))
  (func (export "funcref") (result funcref) (ref.null func))

  (global externref (ref.null extern))
  (global funcref (ref.null func))
)
