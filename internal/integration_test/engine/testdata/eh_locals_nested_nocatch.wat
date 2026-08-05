;; Test: a try_table with NO catch clauses nested inside one WITH catch
;; clauses must not break the locals-save-area tracking.
;;
;; If the inner try_table's end spuriously decrements tryTableDepth,
;; the local.set after it won't emit a store to the save area, and the
;; handler will read a stale value.
(module
  (tag $e)
  (func $thrower (throw $e))
  (func (export "f") (result i32)
    (local $flag i32)
    (local.set $flag (i32.const 1))
    (block $done (result i32)
      (block $catch (result exnref)
        (try_table (catch_all_ref $catch)
          ;; Nested try_table with NO catch clauses (acts as plain block).
          (try_table
            (local.set $flag (i32.const 42))
          )
          ;; Still inside the outer try body — this store must reach
          ;; the save area so the handler sees it.
          (local.set $flag (i32.const 0))
          (call $thrower))
        (unreachable))
      (drop)
      (br $done (local.get $flag)))
  )
)
