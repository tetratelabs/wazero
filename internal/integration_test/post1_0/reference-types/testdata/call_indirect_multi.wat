;; Multi-table call_indirect test case:
;; https://github.com/WebAssembly/spec/blob/bdcf1c1d83d0891a3d852cdd3a01ed34b9060351/test/core/call_indirect.wast#L621-L648

(module
  (type $ii-i (func (param i32 i32) (result i32)))

  (table $t1 funcref (elem $f $g))
  (table $t2 funcref (elem $h $i $j))
  (table $t3 4 funcref)
  (elem (table $t3) (i32.const 0) func $g $h)
  (elem (table $t3) (i32.const 3) func $z)

  (func $f (type $ii-i) (i32.add (local.get 0) (local.get 1)))
  (func $g (type $ii-i) (i32.sub (local.get 0) (local.get 1)))
  (func $h (type $ii-i) (i32.mul (local.get 0) (local.get 1)))
  (func $i (type $ii-i) (i32.div_u (local.get 0) (local.get 1)))
  (func $j (type $ii-i) (i32.rem_u (local.get 0) (local.get 1)))
  (func $z)

  (func (export "call-1") (param i32 i32 i32) (result i32)
    (call_indirect $t1 (type $ii-i) (local.get 0) (local.get 1) (local.get 2))
  )
  (func (export "call-2") (param i32 i32 i32) (result i32)
    (call_indirect $t2 (type $ii-i) (local.get 0) (local.get 1) (local.get 2))
  )
  (func (export "call-3") (param i32 i32 i32) (result i32)
    (call_indirect $t3 (type $ii-i) (local.get 0) (local.get 1) (local.get 2))
  )
)
