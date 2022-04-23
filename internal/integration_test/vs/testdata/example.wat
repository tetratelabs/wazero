;; This example contains currently supported functionality in the text format, used primarily for benchmarking.
(module $example ;; module name
	;; explicit type with param IDs which are to be ignored per WebAssembly/spec#1412
	(type $i32i32_i32 (func (param $i i32) (param $j i32) (result i32)))

    ;; type use by symbolic ID, but no param names
    (import "wasi_snapshot_preview1" "args_sizes_get" (func $wasi.args_sizes_get (type $i32i32_i32)))
	;; type use on an import func which adds param names on anonymous type
	;; TODO correct the param names
	(import "wasi_snapshot_preview1" "fd_write" (func $wasi.fd_write
		(param $fd i32) (param $iovs_ptr i32) (param $iovs_len i32) (param $nwritten_ptr i32) (result i32)))

    ;; func call referencing a func not defined, yet
    (func $call_hello call $hello)

    ;; type use referencing a type not defined, yet
    (func $hello (type 1))
	(type (func))

    ;; start function referencing a function by symbolic ID
	(start $hello)

    ;; export a function before it was defined, given its symbolic ID
    (export "AddInt" (func $addInt))
    ;; export a function before it was defined, with an empty name
    (export "" (func 3))

	;; from https://github.com/summerwind/the-art-of-webassembly-go/blob/main/chapter1/addint/addint.wat
    (func $addInt ;; TODO: function exports (export "AddInt")
        (param $value_1 i32) (param $value_2 i32)
        (result i32)
        local.get 0 ;; TODO: instruction variables $value_1
        local.get 1 ;; TODO: instruction variables $value_2
        i32.add
    )

    ;; export a memory before it was defined, given its symbolic ID
    (export "mem" (memory $mem))
    (memory $mem 1 3)

    ;; add function using "sign-extension-ops"
    ;; https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
    (func (param i64) (result i64) local.get 0 i64.extend16_s)

    ;; add function using "nontrapping-float-to-int-conversion"
    ;; https://github.com/WebAssembly/spec/blob/main/proposals/nontrapping-float-to-int-conversion/Overview.md
    (func (param f32) (result i32) local.get 0 i32.trunc_sat_f32_s)

    ;; add function using "multi-value"
    ;; https://github.com/WebAssembly/spec/blob/main/proposals/multi-value/Overview.md
    (func $swap (param i32 i32) (result i32 i32) local.get 1 local.get 0)
    (export "swap" (func $swap))
)
