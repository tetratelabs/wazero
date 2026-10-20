(assert_invalid
  (module
    ;; When fields are mutable, a subtype's reference fields cannot be subtypes of
    ;; the supertype's fields, they must match exactly.
    (type $a (sub    (struct (field (mut (ref null any))))))
    (type $b (sub $a (struct (field (mut (ref null none))))))
  )
  "sub type 1 does not match super type"
)

(assert_invalid
  (module
    ;; When fields are const, a subtype's reference fields cannot be supertypes of
    ;; the supertype's fields, they must be subtypes.
    (type $a (sub    (struct (field (ref null none)))))
    (type $b (sub $a (struct (field (ref null any)))))
  )
  "sub type 1 does not match super type"
 )

(assert_invalid
  (module
    ;; The mutability of fields must be the same.
    (type $c (sub    (struct (field (mut (ref null any))))))
    (type $d (sub $c (struct (field      (ref null any)))))
  )
  "sub type 1 does not match super type"
)

(assert_invalid
  (module
    ;; The mutability of fields must be the same.
    (type $c (sub    (struct (field      (ref null any)))))
    (type $d (sub $c (struct (field (mut (ref null any))))))
  )
  "sub type 1 does not match super type"
)
