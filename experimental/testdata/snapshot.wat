(module
  (import "example" "snapshot" (func $snapshot (param i32) (result i32)))
  (import "example" "restore" (func $restore (param i32)))

  (func $helper (result i32)
    (call $restore (i32.const 0))
    ;; Not executed
    i32.const 10
  )

  (func (export "run") (result i32) (local i32)
    (call $snapshot (i32.const 0))
    local.set 0
    local.get 0
    (if (result i32)
      (then ;; restore return, finish with the value returned by it
        local.get 0
      )
      (else ;; snapshot return, call heloer
        (call $helper)
      )
    )
  )

  (func (export "snapshot") (param i32) (result i32)
    (call $snapshot (local.get 0))
  )

  (func (export "restore") (param i32)
    (call $restore (local.get 0))
  )

  (memory (export "memory") 1 1)
)
