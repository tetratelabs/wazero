(module
  (func $host_externref (import "host" "externref") (param externref) (result externref))

  (func (export "get_externref_by_host") (result externref)
    (ref.null extern)
    (call $host_externref)
  )
)
