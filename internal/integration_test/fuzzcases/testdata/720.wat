(module
  (func (export "access memory after table.grow") (result i32)
    ref.null extern
    i32.const 10
    table.grow 0
    ;; This should work without any problem,
    ;; and should return non-trivial i32 result.
    i32.load offset=396028 align=1
  )

  ;; Table and memory are as-is produced by fuzzer.
  (table 1 264 externref)
  (memory 10 10)

  ;; Setup the non trivial content on the i32.load
  (data (i32.const 396028) "\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff\ff")
)
