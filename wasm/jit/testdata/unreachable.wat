(module
	(func $cause_unreachable (export "cause_unreachable")
		(call $one)
	)
	(func $one
		(call $two)
	)
	(func $two
		(call $three)
	)
	(func $three
		(unreachable)
	)
)
