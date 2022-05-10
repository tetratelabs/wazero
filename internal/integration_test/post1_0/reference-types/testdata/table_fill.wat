;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/table_fill.wast#L1-L11
(module
  (table $t 10 externref)

  (func (export "fill") (param $i i32) (param $r externref) (param $n i32)
    (table.fill $t (local.get $i) (local.get $r) (local.get $n))
  )

  (func (export "get") (param $i i32) (result externref)
    (table.get $t (local.get $i))
  )
)
