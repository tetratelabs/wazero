;; Test: br exiting a try_table must pop the handler (compiler engine).
;;
;; Without the fix, handler A (stale) incorrectly catches the throw,
;; restoring to handler A's checkpoint and branching to $catchA,
;; which returns 99.
;; With the fix, handler A is popped, and the outer try_table catches
;; the throw, returning 1.
(module
  (tag $tag1 (param))
  (tag $tag2 (param))

  (func (export "test") (result i32)
    ;; Outer: catches $tag1 — CORRECT handler.
    block $outerCatch
      try_table (catch $tag1 $outerCatch)

        ;; try_table A: catches $tag1 with target $catchA.
        ;; We exit via br, leaving handler A stale.
        block $catchA
          block $skipA
            try_table (catch $tag1 $catchA)
              br $skipA  ;; exits try_table A without End
            end
            unreachable
          end
          ;; After $skipA: handler A still on stack (bug).

          ;; try_table B: catches $tag2, but we throw $tag1.
          block $catchB
            try_table (catch $tag2 $catchB)
              throw $tag1
            end
            unreachable
          end
          ;; caught by B (wrong — B catches tag2, not tag1)
          i32.const 2
          return
        end
        ;; $catchA: stale handler A jumped here — WRONG result
        i32.const 99
        return

      end
      ;; fell through outer try_table
      i32.const 3
      return
    end
    ;; $outerCatch: outer caught $tag1 — CORRECT
    i32.const 1
  )
)
