(module
  (func (export "main")
    (param f32) ;; unused param -> wazeroir always emits the drop operation on this.
    (result ;; 100 results (interms of uint64 representations.
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 20
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 40
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 60
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 80
      i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 i64 ;; 100
    )
    (i64.const 1)(i64.const 2)(i64.const 3)(i64.const 4)(i64.const 5)(i64.const 6)(i64.const 7)(i64.const 8)(i64.const 9)(i64.const 10) ;; 10
    (i64.const 11)(i64.const 12)(i64.const 13)(i64.const 14)(i64.const 15)(i64.const 16)(i64.const 17)(i64.const 18)(i64.const 19)(i64.const 20) ;; 20
    (i64.const 21)(i64.const 22)(i64.const 23)(i64.const 24)(i64.const 25)(i64.const 26)(i64.const 27)(i64.const 28)(i64.const 29)(i64.const 30) ;; 30
    (i64.const 31)(i64.const 32)(i64.const 33)(i64.const 34)(i64.const 35)(i64.const 36)(i64.const 37)(i64.const 38)(i64.const 39)(i64.const 40) ;; 40
    (i64.const 41)(i64.const 42)(i64.const 43)(i64.const 44)(i64.const 45)(i64.const 46)(i64.const 47)(i64.const 48)(i64.const 49)(i64.const 50) ;; 50
    (i64.const 51)(i64.const 52)(i64.const 53)(i64.const 54)(i64.const 55)(i64.const 56)(i64.const 57)(i64.const 58)(i64.const 59)(i64.const 60) ;; 60
    (i64.const 61)(i64.const 62)(i64.const 63)(i64.const 64)(i64.const 65)(i64.const 66)(i64.const 67)(i64.const 68)(i64.const 69)(i64.const 70) ;; 70
    (i64.const 71)(i64.const 72)(i64.const 73)(i64.const 74)(i64.const 75)(i64.const 76)(i64.const 77)(i64.const 78)(i64.const 79)(i64.const 80) ;; 80
    (i64.const 81)(i64.const 82)(i64.const 83)(i64.const 84)(i64.const 85)(i64.const 86)(i64.const 87)(i64.const 88)(i64.const 89)(i64.const 90) ;; 90
    (i64.const 91)(i64.const 92)(i64.const 93)(i64.const 94)(i64.const 95)(i64.const 96)(i64.const 97)(i64.const 98)(i64.const 99)(i64.const 100) ;; 100
  )
)
