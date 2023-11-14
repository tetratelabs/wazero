(module
  (func (result f64 f64)
    block (result f64 f64)  ;; label = @1
      global.get 0
      block (result f64 f64)  ;; label = @2
        global.get 0
        f64.const 1.123
        i32.const 2
        br_table 0 (;@2;) 1 (;@1;) 2 (;@0;) 2 (;@0;)
      end
      drop
    end
  )
  (memory (;0;) 0 2)
  (global (mut f64) f64.const -nan:0xf94ffffffffff (;=NaN;))
  (export "" (func 0))
  (export "1" (memory 0))
)
