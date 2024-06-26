(module
  (memory (export "memory")  1 5) ;; start with one memory page, and max of 4 pages
  (func $main

   (loop $my_loop
      (i32.gt_s
         (memory.grow (i32.const 1))
         (i32.const 0))
      br_if $my_loop
      unreachable)
    return
  )
  (start $main)
)