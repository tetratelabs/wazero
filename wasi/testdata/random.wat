;; This is a wat file to just export clock WASI API to the host environment for testing the APIs.
;; This is currently separated as a wat file and pre-compiled because our text parser doesn't
;; implement 'memory' yet. After it supports 'memory', we can remove this file and embed this
;; wat file in the Go test code.
;;
;; Note: Although this is a raw wat file which should be moved under /tests/wasi in principle,
;; this file is put here for now, because this is a temporary file until the parser supports
;; the enough syntax, and this file will be embedded in unit test codes after that.
(module
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get  (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))
  (memory 1)  ;; just an arbitrary size big enough for tests
  (export "memory" (memory 0))
  ;; Define wrapper functions instead of just exporting the imported WASI APIS for now
  ;; because wazero's interpreter has a bug that it crashes when an imported-and-exported host function
  ;; is called from the host environment, which will be fixed soon.
  ;; After it's fixed, these wrapper functions are no longer necessary.
  (func $random_get (param i32 i32) (result i32)
        local.get 0
        local.get 1
        call $wasi.random_get
        )
  (export "random_get" (func $random_get))
  )