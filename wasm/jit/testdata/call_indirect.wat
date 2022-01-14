(module
  (type $T (func (param) (result i32)))
  (type $U (func (param) (result i32)))
  (table funcref (elem $t1 $t2 $t3 $u1 $u2 $t1 $t3))

  (func $t1 (type $T) (i32.const 1))
  (func $t2 (type $T) (i32.const 2))
  (func $t3 (type $T) (i32.const 3))
  (func $u1 (type $U) (i32.const 4))
  (func $u2 (type $U) (i32.const 5))

  (func (export "callt") (param $i i32) (result i32)
    (call_indirect (type $T) (local.get $i))
  )

  (func (export "callu") (param $i i32) (result i32)
    (call_indirect (type $U) (local.get $i))
  )
)
