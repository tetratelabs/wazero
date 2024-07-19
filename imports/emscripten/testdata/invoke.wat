(module
  (type $0 (func (param i32)))
  (import "env" "invoke_i" (func $invoke_i (param i32) (result i32)))
  (import "env" "invoke_ii" (func $invoke_ii (param i32 i32) (result i32)))
  (import "env" "invoke_iii" (func $invoke_iii (param i32 i32 i32) (result i32)))
  (import "env" "invoke_iiii" (func $invoke_iiii (param i32 i32 i32 i32) (result i32)))
  (import "env" "invoke_iiiii" (func $invoke_iiiii (param i32 i32 i32 i32 i32) (result i32)))
  (import "env" "invoke_v" (func $invoke_v (param i32)))
  (import "env" "invoke_vi" (func $invoke_vi (param i32 i32)))
  (import "env" "invoke_vii" (func $invoke_vii (param i32 i32 i32)))
  (import "env" "invoke_viii" (func $invoke_viii (param i32 i32 i32 i32)))
  (import "env" "invoke_viiii" (func $invoke_viiii (param i32 i32 i32 i32 i32)))
  (import "env" "_emscripten_throw_longjmp" (func $_emscripten_throw_longjmp))

  (table 22 22 funcref)

  (global $__stack_pointer (mut i32) (i32.const 65536))
  (func $stackSave (export "emscripten_stack_get_current") (result i32)
    global.get $__stack_pointer)
  (func $stackRestore (export "_emscripten_stack_restore") (param i32)
    local.get 0
    global.set $__stack_pointer)
  (func $setThrew (export "setThrew") (param i32 i32))

  (func $v_i32 (result i32) (i32.const 42))
  (func $v_i32_unreachable (result i32) unreachable)

  (elem (i32.const 0) $v_i32 $v_i32_unreachable)

  ;; call_v_i32 should be called with 0 or 1 and expect 42 or unreachable.
  (func $call_v_i32 (export "call_v_i32") (param i32) (result i32)
    (call $invoke_i (local.get 0)))

  (func $i32_i32 (param i32) (result i32) (local.get 0))
  (func $i32_i32_unreachable (param i32) (result i32) unreachable)

  (elem (i32.const 2) $i32_i32 $i32_i32_unreachable)

  ;; call_i32_i32 should be called with 2 or 3 followed by one number which is
  ;; the result on $0 == 2 or unreachable on 3.
  (func $call_i32_i32 (export "call_i32_i32") (param i32 i32) (result i32)
    (call $invoke_ii (local.get 0) (local.get 1)))

  (func $i32i32_i32 (param i32 i32) (result i32) (i32.add (local.get 0) (local.get 1)))
  (func $i32i32_i32_unreachable (param i32 i32) (result i32) unreachable)

  (elem (i32.const 4) $i32i32_i32 $i32i32_i32_unreachable)

  ;; call_i32i32_i32 should be called with 4 or 5 followed by two numbers
  ;; whose sum is the result on $0 == 4 or unreachable on 5.
  (func $call_i32i32_i32 (export "call_i32i32_i32") (param i32 i32 i32) (result i32)
    (call $invoke_iii (local.get 0) (local.get 1) (local.get 2)))

  (func $i32i32i32_i32 (param i32 i32 i32) (result i32)
    (i32.add (i32.add (local.get 0) (local.get 1)) (local.get 2)))
  (func $i32i32i32_i32_unreachable (param i32 i32 i32) (result i32) unreachable)

  (elem (i32.const 6) $i32i32i32_i32 $i32i32i32_i32_unreachable)

  ;; call_i32i32i32_i32 should be called with 6 or 7 followed by three numbers
  ;; whose sum is the result on $0 == 6 or unreachable on 7.
  (func $call_i32i32i32_i32 (export "call_i32i32i32_i32") (param i32 i32 i32 i32) (result i32)
    (call $invoke_iiii (local.get 0) (local.get 1) (local.get 2) (local.get 3)))

  (func $i32i32i32i32_i32 (param i32 i32 i32 i32) (result i32)
    (i32.add (i32.add (i32.add (local.get 0) (local.get 1)) (local.get 2)) (local.get 3)))
  (func $i32i32i32i32_i32_unreachable (param i32 i32 i32 i32) (result i32) unreachable)

  (elem (i32.const 8) $i32i32i32i32_i32 $i32i32i32i32_i32_unreachable)

  ;; calli32_i32i32i32i32_i32 should be called with 8 or 9 followed by four numbers
  ;; whose sum is the result on $0 == 8 or unreachable on 9.
  (func $calli32_i32i32i32i32_i32 (export "calli32_i32i32i32i32_i32") (param i32 i32 i32 i32 i32) (result i32)
    (call $invoke_iiiii (local.get 0) (local.get 1) (local.get 2) (local.get 3) (local.get 4)))

  (func $v_v)
  (func $v_v_unreachable unreachable)

  (elem (i32.const 10) $v_v $v_v_unreachable)

  ;; call_v_v should be called with 10 or 11 and expect unreachable on 11.
  (func $call_v_v (export "call_v_v") (param i32)
    (call $invoke_v (local.get 0)))

  (func $i32_v (param i32))
  (func $i32_v_unreachable (param i32) unreachable)

  (elem (i32.const 12) $i32_v $i32_v_unreachable)

  ;; call_i32_v should be called with 12 or 13 followed by one number and
  ;; expect unreachable on 2.
  (func $call_i32_v (export "call_i32_v") (param i32 i32)
    (call $invoke_vi (local.get 0) (local.get 1)))

  (func $i32i32_v (param i32 i32))
  (func $i32i32_v_unreachable (param i32 i32) unreachable)

  (elem (i32.const 14) $i32i32_v $i32i32_v_unreachable)

  ;; call_i32i32_v should be called with 14 or 15 followed by two numbers
  ;; and expect unreachable on 15.
  (func $call_i32i32_v (export "call_i32i32_v") (param i32 i32 i32)
    (call $invoke_vii (local.get 0) (local.get 1) (local.get 2)))

  (func $i32i32i32_v (param i32 i32 i32))
  (func $i32i32i32_v_unreachable (param i32 i32 i32) unreachable)

  (elem (i32.const 16) $i32i32i32_v $i32i32i32_v_unreachable)

  ;; call_i32i32i32_v should be called with 16 or 17 followed by three numbers
  ;; and expect unreachable on 17.
  (func $call_i32i32i32_v (export "call_i32i32i32_v") (param i32 i32 i32 i32)
    (call $invoke_viii (local.get 0) (local.get 1) (local.get 2) (local.get 3)))

  (func $i32i32i32i32_v (param i32 i32 i32 i32))
  (func $i32i32i32i32_v_unreachable (param i32 i32 i32 i32) unreachable)

  (elem (i32.const 18) $i32i32i32i32_v $i32i32i32i32_v_unreachable)

  ;; calli32_i32i32i32i32_v should be called with 18 or 19 followed by four
  ;; numbers and expect unreachable on 19.
  (func $calli32_i32i32i32i32_v (export "calli32_i32i32i32i32_v") (param i32 i32 i32 i32 i32)
    (call $invoke_viiii (local.get 0) (local.get 1) (local.get 2) (local.get 3) (local.get 4)))

  (func $call_longjmp_throw
    (call $_emscripten_throw_longjmp)
    (global.set $__stack_pointer (i32.const 43)))

  (func $call_longjmp_throw_unreachable unreachable)

  (elem (i32.const 20) $call_longjmp_throw $call_longjmp_throw_unreachable)

  ;; $call_invoke_v_with_longjmp_throw should be called with 20 or 21 and
  ;; expect unreachable on 21. $call_invoke_v_with_longjmp_throw mimics
  ;; Emscripten by setting the stack pointer to a different value than default.
  ;; We ensure that the stack pointer was not changed to 43 by $call_longjump.
  (func $call_invoke_v_with_longjmp_throw (export "call_invoke_v_with_longjmp_throw") (param i32)
    (global.set $__stack_pointer (i32.const 42))
    (call $invoke_v (local.get 0))
    global.get $__stack_pointer
    i32.const 42
    i32.ne
    (if (then unreachable))
  )
)
