;; Test: locals mutated inside a try_table body must retain their
;; throw-time values when an exception is caught.
;;
;; Bug: wazevo clones the stack (including spill slots holding locals)
;; on try_table entry. When a throw occurs, the cloned stack is restored,
;; reverting locals to their try_table-entry values instead of the
;; throw-time values required by the spec.
(module
  (tag $e)
  (func $thrower (throw $e))
  (func (export "f") (result i32)
    (local $flag i32)
    (local.set $flag (i32.const 1))        ;; flag = 1 at try_table entry
    (block $done (result i32)
      (block $catch (result exnref)
        (try_table (catch_all_ref $catch)
          (local.set $flag (i32.const 0))  ;; flag = 0 inside the try body
          (call $thrower))                 ;; throws; caught by catch_all_ref
        (unreachable))
      (drop)                               ;; $catch: drop the exnref
      (br $done (local.get $flag)))        ;; return flag — must be 0
  )
)
