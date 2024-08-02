
(module
	(import "inoutdispatcher" "dispatch" (func $dispatch))
    (func (export "dispatch")
        (call $dispatch)
    )
)
