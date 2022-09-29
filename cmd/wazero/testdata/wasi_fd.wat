;; $wasi_fd is a WASI command which reads from bear.txt
(module $wasi_fd
	;; path_open returns a file descriptor to a path
	;;
	;; See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-args_sizes_get---errno-size-size
    (import "wasi_snapshot_preview1" "path_open"
        (func $wasi.path_open (param $fd i32) (param $dirflags i32) (param $path i32) (param $path_len i32) (param $oflags i32) (param $fs_rights_base i64) (param $fs_rights_inheriting i64) (param $fdflags i32) (param $result.opened_fd i32) (result (;errno;) i32)))

    ;; fd_read reads bytes from a file descriptor.
    ;;
    ;; See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_read
    (import "wasi_snapshot_preview1" "fd_read"
        (func $wasi.fd_read (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))

    ;; fd_write write bytes to a file descriptor.
    ;;
    ;; See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#fd_write
    (import "wasi_snapshot_preview1" "fd_write"
        (func $wasi.fd_write (param $fd i32) (param $iovs i32) (param $iovs_len i32) (param $result.size i32) (result (;errno;) i32)))

    ;; WASI commands are required to export "memory". Particularly, imported functions mutate this.
    ;;
    ;; Note: 1 is the size in pages (64KB), not bytes!
    ;; See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memories%E2%91%A7
    (memory (export "memory") 1)

    ;; $iovs are offset/length pairs in memory fd_write copies to the file descriptor.
    ;; $main will only write one offset/length pair, corresponding to a read buffer
    (global $iovs i32 i32.const 1024) ;; 1024 is an arbitrary offset

    ;; WASI parameters are usually memory offsets, you can ignore values by writing them to an unread offset.
    (global $ignored i32 i32.const 32768)

    ;; _start is a special function defined by a WASI Command that runs like a main function would.
    ;;
    ;; See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
    (func $main (export "_start")
        ;; First open the path
        (call $wasi.path_open
            (i32.const 3) ;; fd of fs root
            (i32.const 0) ;; ignore $dirflags
            (i32.const 5000) ;; path is in data
            (i32.const 8) ;; path len
            (i32.const 0) ;; ignore $oflags
            (i64.const 0) ;; ignore $fs_rights_base
            (i64.const 0) ;; ignore $fs_rights_inheriting
            (i32.const 0) ;; ignore $fdflags
            (i32.const 0) ;; write result fd to memory offset 0
        )
        drop ;; ignore the errno returned

        ;; set iovs to a 50 byte read buffer at offset 100
        (i32.store (global.get $iovs) (i32.const 100))
        (i32.store (i32.add (global.get $iovs) (i32.const 4)) (i32.const 50))

        (call $wasi.fd_read
            (i32.load (i32.const 0)) ;; load from offset 0 which has fd of file
            (global.get $iovs) ;; $iovs is the start offset of the IO vectors to read into.
            (i32.const 1) ;; $iovs_len is the count of offset/length pairs to read from iovs
            (i32.add (global.get $iovs) (i32.const 4)) ;; set number of bytes read as buffer length for fd_write below
        )
        drop ;; ignore the errno returned

        ;; Finally, write the memory region to the file.
        (call $wasi.fd_write
            (i32.const 1) ;; $fd is a file descriptor and 1 is stdout (console).
            (global.get $iovs) ;; $iovs is the start offset of the IO vectors to copy.
            (i32.const 1) ;; $iovs_len is the count of offset/length pairs to copy to memory.
            (global.get $ignored) ;; ignore $result.size as we aren't verifying it.
        )
        drop ;; ignore the errno returned
    )
    (data $.rodata (i32.const 5000) "bear.txt")
)
