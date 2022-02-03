;; This is a module to test that the engine can call "imported-and-then-exported-back" function correctly
(module
  ;; arbitrary function with params
  (import "env" "add_int" (func $add_int (param i32 i32) (result i32)))
  ;; add_int is imported from the environment, but it's also exported back to the environment
  (export "add_int" (func $add_int))
  )
