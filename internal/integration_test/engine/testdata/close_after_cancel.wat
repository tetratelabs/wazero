;; Calls the host to cancel its context, then exits, traps or overflows the
;; stack before reaching any termination check.
(module
  (import "host" "cancel" (func $cancel))
  (import "wasi_snapshot_preview1" "proc_exit" (func $proc_exit (param i32)))
  (memory 1)
  (func $recurse (call $recurse))
  (func (export "exit") (call $cancel) (call $proc_exit (i32.const 0)))
  (func (export "trap") (call $cancel) unreachable)
  (func (export "overflow") (call $cancel) (call $recurse)))
