(module
  (tag $e)
  ;; T is exited by `br $T` (a branch to the try_table's own label). Its handler
  ;; must be popped. The subsequent `throw` must then propagate to the OUTER
  ;; handler (-> 1). If T's leave was skipped, the stale T catches it (-> 0).
  (func (export "run") (result i32)
    (block $ok (result exnref)
      (try_table $TO (catch_all_ref $ok)
        (block $h (result exnref)
          (try_table $T (catch_all_ref $h)
            (br $T))
          (throw $e))             ;; T popped -> TO catches; T stale -> T catches
        (drop)
        (return (i32.const 0)))   ;; INCORRECT: stale T caught it
      (unreachable))
    (drop)
    (i32.const 1)))               ;; CORRECT: throw propagated past popped T
