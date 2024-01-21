(module
  (func (;0;)
    (local i32)
      loop  ;; label = @4
        loop  ;; label = @5
          global.get 0
          i32.eqz
          if  ;; label = @6
            unreachable
          end
          global.get 0
          i32.const 1
          i32.sub
          global.set 0
          block ;; label = @6
            i32.const 0
            v128.load32_splat offset=32768
            i32x4.extract_lane 2
            i64.load offset=32884
            br 2 (;@5;)
          end
        end
    end
  )
  (memory (;0;) 1 6)
  (global (;0;) (mut i32) i32.const 1000)
  (export "" (func 0))
  (data (;5;) (i32.const 0) "\8b\8b\8b\8b\8b\96")
)
