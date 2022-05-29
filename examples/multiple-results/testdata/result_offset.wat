;; result-offset/wasm defines Wasm functions that illustrate multiple results
;; using a technique compatible with any WebAssembly 1.0 runtime.
;;
;; To return a value in WASM written to a result parameter, you have to define
;; memory and pass a location to write the result. At the end of your function,
;; you load that location.
(module $result-offset/wasm
  ;; To use result parameters, we need scratch memory. Allocate the least
  ;; possible: 1 page (64KB).
  (memory 1 1)

  ;; get_age returns a result, while a second result is written to memory.
  (func $get_age (param $result_offset.age i32) (result (;errno;) i32)
    local.get 0   ;; stack = [$result_offset.age]
    i64.const 37  ;; stack = [$result_offset.age, 37]
    i64.store     ;; stack = []
    i32.const 0   ;; stack = [0]
  )

  ;; Now, define a function that shows the Wasm mechanics returning something
  ;; written to a result parameter.
  ;; The caller provides a memory offset to the callee, so that it knows where
  ;; to write the second result.
  (func (export "call_get_age") (result i64)
    i32.const 8   ;; stack = [8] arbitrary $result_offset.age param to get_age
    call $get_age ;; stack = [errno] result of get_age
    drop          ;; stack = []

    i32.const 8   ;; stack = [8] same value as the $result_offset.age parameter
    i64.load      ;; stack = [age]
  )
)
