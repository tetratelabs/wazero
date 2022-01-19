(module
  (import "env" "host_func" (func $host_func ))

  (func $main (param i32)
    block  ;; label = @1
      loop  ;; label = @2
        local.get 0
        i32.eqz
        br_if 1 (;@1;)
        local.get 0
        i32.const -1
        i32.add
        local.set 0
        call $host_func
        br 0 (;@2;)
      end
    end
  )

  (func $called_by_host_func (result i32)
    i32.const 100
  )

  (export "main" (func $main))
  (export "called_by_host_func" (func $called_by_host_func))

)
