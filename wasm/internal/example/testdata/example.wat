;; example has a work-in-progress of supported functionality, used primarily for benchmarking. This includes:
;; * module and function names
;; * explicit, and inlined type definitions (including anonymous)
;; * inlined and overridden type parameter names
;; * start function
(module $example
	(type $i32i32_i32 (func (param i32 i32) (result i32)))
	(import "wasi_snapshot_preview1" "args_sizes_get" (func $runtime.args_sizes_get (type $i32i32_i32)))
	(import "wasi_snapshot_preview1" "fd_write" (func $runtime.fd_write
		(param $fd i32) (param $iovs_ptr i32) (param $iovs_len i32) (param $nwritten_ptr i32) (result i32)))
	(import "Math" "Mul" (func $mul (param $x f32) (param $y f32) (result f32)))
	(import "Math" "Add" (func $add (type $i32i32_i32) (param $l i32) (param $r i32) (result i32)))
	(type (func))
	(import "" "hello" (func $hello (type 1)))
	;; re-export a.k.a. proxy a function!
    (export "args_sizes_get" (func $runtime.args_sizes_get))
	(start $hello)
)
