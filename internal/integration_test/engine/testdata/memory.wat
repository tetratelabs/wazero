(module $memory
  (func $grow (param $delta i32) (result (;previous_size;) i32) local.get 0 memory.grow)
  (func $size (result (;size;) i32) memory.size)

  (memory 0)

  (export "size" (func $size))
  (export "grow" (func $grow))
  (export "memory" (memory 0))

  (func $store (param $offset i32)
	local.get 0    ;; memory offset
	i64.const 1
	i64.store
  )
  (export "store" (func $store))
)
