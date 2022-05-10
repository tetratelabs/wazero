;; https://github.com/WebAssembly/spec/blob/1ffb924e94856e89f787fc2000fa4c5dc069a24f/test/core/table_set.wast#L101-L107
(module
  (table $t1 1 externref)
  (table $t2 1 funcref)
  (func $type-value-externref-vs-funcref-multi (param $r externref)
    (table.set $t2 (i32.const 0) (local.get $r))
  )
)
