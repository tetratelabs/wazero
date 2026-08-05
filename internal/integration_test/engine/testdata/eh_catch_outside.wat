(module
  (tag $e)
  ;; TI's catch (catch_all $L) jumps to $L, which is OUTSIDE the enclosing TO.
  ;; That jump must pop TO. The 2nd throw must then reach the OUTER handler
  ;; (-> 1). If TO was left stale, it catches the 2nd throw instead (-> 0).
  (func (export "run") (result i32)
    (block $ok (result exnref)
      (try_table $TOUTER (catch_all_ref $ok)
        (block $L
          (block $hO (result exnref)
            (try_table $TO (catch_all_ref $hO)
              (try_table $TI (catch_all $L)
                (throw $e))        ;; 1st throw -> TI catches -> after $L (exits TO)
              (unreachable))
            (unreachable))
          (drop)
          (return (i32.const 0)))  ;; INCORRECT: stale TO caught the 2nd throw
        (throw $e))                ;; 2nd throw -> TO popped: TOUTER; stale: TO
      (unreachable))
    (drop)
    (i32.const 1)))                ;; CORRECT
