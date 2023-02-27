(module $infinite_loop
  (func $main (export "_start")
    (loop
      br 0
    )
  )
)
