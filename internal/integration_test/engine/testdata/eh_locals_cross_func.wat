;; Test: cross-function try_table nesting — function A has a try_table,
;; calls function B which also has a try_table. B throws an exception
;; that A catches. A's handler must see A's local values from before
;; the call to B, not B's locals.
(module
  (tag $tA)
  (tag $tB)

  (func $inner
    (local $x i32)
    (local.set $x (i32.const 99))
    (block $catch
      (try_table (catch $tB $catch)
        (local.set $x (i32.const 77))
        (throw $tA)))          ;; throws $tA — not caught by inner
    (unreachable))

  (func (export "f") (result i32)
    (local $flag i32)
    (local.set $flag (i32.const 1))
    (block $done (result i32)
      (block $catch (result exnref)
        (try_table (catch_all_ref $catch)
          (local.set $flag (i32.const 0))
          (call $inner))       ;; inner throws $tA, outer catches
        (unreachable))
      (drop)
      (br $done (local.get $flag)))   ;; must be 0 (A's throw-time value)
  )
)
