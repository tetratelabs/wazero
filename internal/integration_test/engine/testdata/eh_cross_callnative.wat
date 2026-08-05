;; Reproducer: try_table in grandparent, exception thrown in grandchild,
;; caught by grandparent's handler via cross-frame doRestore.
;; The `eh` branch interpreter bug: after doRestore removes the parent frame,
;; parent's callNativeFunc wrongly continues executing grandparent's body.
(module
  (tag $e0 (param))

  ;; grandchild: just throws
  (func $grandchild
    throw $e0
  )

  ;; child: calls grandchild (no try_table of its own)
  ;; exception propagates out of child to grandparent's handler
  (func $child
    call $grandchild
  )

  ;; grandparent: has a try_table, calls child
  ;; When child's exception is caught by grandparent's handler,
  ;; doRestore removes child's frame from ce.frames.
  ;; child's callNativeFunc (running in callWithUnwind) must return,
  ;; not continue executing grandparent's body.
  (func $grandparent (result i32)
    block $caught
      try_table (catch_all $caught)
        call $child
      end
      ;; No exception (unreachable in this test)
      i32.const -1
      return
    end
    ;; Exception caught: return 1
    i32.const 1
  )

  (func (export "test_cross_frame_catch") (result i32)
    call $grandparent
  )

  ;; More realistic pdfium-style: grandparent has catch_all_ref,
  ;; child has inner catch_all_ref + throw_ref, exception re-caught by grandparent
  (func $child_rethrow
    (local $x exnref)
    block $done
      block $catch (result exnref)
        try_table (catch_all_ref $catch)
          call $grandchild
        end
        br $done
      end
      local.set $x
      local.get $x
      throw_ref  ;; rethrow: grandparent's handler must catch this
    end
  )

  (func $grandparent_catches_rethrow (result i32)
    (local $x exnref)
    block $done
      block $catch (result exnref)
        try_table (catch_all_ref $catch)
          call $child_rethrow
        end
        br $done
      end
      local.set $x
    end
    i32.const 2  ;; caught the rethrow
  )

  (func (export "test_rethrow_cross_frame") (result i32)
    call $grandparent_catches_rethrow
  )
)
