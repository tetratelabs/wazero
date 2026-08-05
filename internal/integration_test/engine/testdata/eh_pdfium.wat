;; Tests pdfium/Emscripten EH pattern:
;; - throw in leaf
;; - catch_all_ref + throw_ref (Emscripten cleanup/rethrow pattern) at multiple call levels
;; - cross-frame exception propagation
(module
  (tag $e0 (param))

  ;; leaf: throws
  (func $leaf
    throw $e0
  )

  ;; level2: catch_all_ref, cleanup, rethrow — like pdfium's inner destructor wrapper
  (func $level2
    (local $x exnref)
    block $done
      block $catch (result exnref)
        try_table (catch_all_ref $catch)
          call $leaf
        end
        br $done
      end
      local.set $x
      ;; cleanup ...
      local.get $x
      throw_ref
    end
  )

  ;; level1: outer catch_all_ref, calls level2, then itself rethrows
  ;; This tests: inner catch_all_ref -> throw_ref -> outer catch_all_ref -> throw_ref
  (func $level1
    (local $x exnref)
    block $done
      block $catch (result exnref)
        try_table (catch_all_ref $catch)
          call $level2
        end
        br $done
      end
      local.set $x
      ;; outer cleanup ...
      local.get $x
      throw_ref
    end
  )

  ;; top: calls level1 and catches with catch_all
  (func (export "test_two_level_rethrow") (result i32)
    block $caught
      try_table (catch_all $caught)
        call $level1
      end
      i32.const -1
      return
    end
    i32.const 1
  )

  ;; Verify the leaf throws are caught correctly at each level
  (func (export "test_one_level_rethrow") (result i32)
    block $caught
      try_table (catch_all $caught)
        call $level2
      end
      i32.const -1
      return
    end
    i32.const 1
  )
)
