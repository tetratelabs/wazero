;; Hypothesis: br_if inside try_table body that exits the try_table block
;; skips the popTryHandler at the continuation label, leaving an orphaned handler.
;; The orphaned handler then incorrectly catches a later exception.
(module
  (tag $e0)

  ;; This function has a try_table wrapping a loop.
  ;; When br_if fires to exit the loop, it also exits the try_table
  ;; without going through try_table's `end` (so popTryHandler is skipped).
  (func $loop_with_try (param $n i32)
    block $outer                       ;; $outer is the catch target
      try_table (catch_all $outer)     ;; handler pushed, targets $outer end
        loop $loop
          local.get $n
          i32.eqz
          br_if $outer                 ;; exits try_table body — skips popTryHandler!
          local.get $n
          i32.const 1
          i32.sub
          local.set $n
          br $loop
        end
        ;; only reached if n was 0 from the start; popTryHandler runs
      end  ;; try_table end: emits popTryHandler (only on fall-through path)
    end    ;; $outer
  )

  (func $throw
    throw $e0
  )

  (func (export "test") (result i32)
    ;; Call loop_with_try — leaves orphaned handler in ce.tryHandlers
    i32.const 3
    call $loop_with_try

    ;; Now throw inside a proper try_table.
    ;; Correct: caught by the inner try_table → returns 1.
    ;; Bug: caught by the orphaned handler from loop_with_try → wrong result.
    block $done
      block $catch
        try_table (catch_all $catch)
          call $throw
        end
        br $done
      end
      ;; exception caught — expected path
      i32.const 1
      return
    end
    ;; no exception — shouldn't happen
    i32.const 0
  )
)
