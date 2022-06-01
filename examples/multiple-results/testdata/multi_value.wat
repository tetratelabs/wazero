;; multiValueWasmFunctions defines Wasm functions that illustrate multiple
;; results using the "multiple-results" feature.
(module $multi-value/wasm

  ;; Define a function that returns two results
  (func $get_age (result (;age;) i64 (;errno;) i32)
    i64.const 37  ;; stack = [37]
    i32.const 0   ;; stack = [37, 0]
  )

  ;; Now, define a function that returns only the first result.
  (func (export "call_get_age") (result i64)
    call $get_age ;; stack = [37, errno] result of get_age
    drop          ;; stack = [37]
  )
)
