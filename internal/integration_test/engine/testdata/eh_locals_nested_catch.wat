;; Test: nested try_tables in the same function where an exception thrown
;; in the inner body is caught by the outer handler. The outer handler
;; must see the throw-time local value set inside the inner body.
(module
  (tag $t1)
  (tag $t2)
  (func $thrower (throw $t1))
  (func (export "f") (result i32)
    (local $flag i32)
    (local.set $flag (i32.const 1))
    (block $done (result i32)
      (block $outerCatch (result exnref)
        (try_table (catch_all_ref $outerCatch)
          ;; Inner try_table catches $t2 only — $t1 propagates to outer.
          (block $innerCatch
            (try_table (catch $t2 $innerCatch)
              (local.set $flag (i32.const 0))
              (call $thrower)))          ;; throws $t1, not caught by inner
          (unreachable))
        (unreachable))
      (drop)
      (br $done (local.get $flag)))      ;; must be 0, not 1
  )
)
