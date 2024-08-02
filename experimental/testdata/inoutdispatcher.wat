(module
    (import "wasi_snapshot_preview1" "fd_read" (func $fd_read (param i32 i32 i32 i32) (result i32)))
    (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))
    (memory 1 1 )
    (func (export "dispatch")
        ;; Buffer of 100 chars to read into.
        (i32.store (i32.const 4) (i32.const 12))
        (i32.store (i32.const 8) (i32.const 100))

        (block $done
            (loop $read
                ;; Read from stdin.
                (call $fd_read
                    (i32.const 0) ;; fd; 0 is stdin.
                    (i32.const 4) ;; iovs
                    (i32.const 1) ;; iovs_len
                    (i32.const 8) ;; nread
                )

                ;; If nread is 0, we're done.
                (if (i32.eq (i32.load (i32.const 8)) (i32.const 0))
                    (then br $done)
                )

                ;; Write to stdout.
                (drop (call $fd_write
                    (i32.const 1) ;; fd; 1 is stdout.
                    (i32.const 4) ;; iovs
                    (i32.const 1) ;; iovs_len
                    (i32.const 0) ;; nwritten
                ))
                (br $read)

            )
        )
    )

)
