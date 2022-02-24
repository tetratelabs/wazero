(module
	(import "host" "cause_unreachable" (func $cause_unreachable ))

	(func $main (export "main")
		(call $one)
	)
	(func $one
		(call $two)
	)
	(func $two
		(call $cause_unreachable)
	)
)
