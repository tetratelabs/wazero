;; multiValueWasmFunctions defines Wasm functions that illustrate multiple
;; results using the "multiple-results" feature.
(module $multi-value/imported_host
  ;; Imports the `get_age` function from `multi-value/host` defined in the host.
  (func $get_age (import "multi-value/host" "get_age") (result (;age;) i64 (;errno;) i32))

  ;; Now, define a function that returns only the first result.
  (func (export "call_get_age") (result i64)
    call $get_age ;; stack = [37, errno] result of get_age
    drop          ;; stack = [37]
  )
)
