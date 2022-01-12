(module
  (memory 1)

  (func $i16_store_little (param $address i32) (param $value i32) 
    (i32.store8 (i32.add (local.get $address) (i32.const 1)) (i32.shr_u (local.get $value) (i32.const 8)))
  )
)
