;; Test that throw_ref on a null exnref traps as "null reference" not "unreachable".
;;
;; The null reaches throw_ref as a function parameter, but the host is not the one supplying
;; it: an exnref in an exported signature makes a function uncallable from the host, since a
;; handle only names an exception inside the call that produced it. So the exported wrapper
;; takes nothing and makes the null itself.
(module
  (func $throw_ref_param (param exnref)
    local.get 0
    throw_ref
  )
  (func (export "throw_ref_null")
    ref.null exn
    call $throw_ref_param
  )
)
