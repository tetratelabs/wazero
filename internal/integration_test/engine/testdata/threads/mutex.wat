(module
  (memory 1 1 shared)

  (func $tryLockMutex32
    (param $mutexAddr i32) (result i32)
    ;; Attempt to grab the mutex. The cmpxchg operation atomically
    ;; does the following:
    ;; - Loads the value at $mutexAddr.
    ;; - If it is 0 (unlocked), set it to 1 (locked).
    ;; - Return the originally loaded value.
    (i32.atomic.rmw.cmpxchg
      (local.get $mutexAddr) ;; mutex address
      (i32.const 0)          ;; expected value (0 => unlocked)
      (i32.const 1))         ;; replacement value (1 => locked)

    ;; The top of the stack is the originally loaded value.
    ;; If it is 0, this means we acquired the mutex. We want to
    ;; return the inverse (1 means mutex acquired), so use i32.eqz
    ;; as a logical not.
    (i32.eqz)
  )

  ;; Lock a mutex at the given address, retrying until successful.
  (func $lockMutex32
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        ;; Try to lock the mutex. $tryLockMutex returns 1 if the mutex
        ;; was locked, and 0 otherwise.
        (call $tryLockMutex32 (local.get $mutexAddr))
        (br_if $done)

        ;; Wait for the other agent to finish with mutex.
        (memory.atomic.wait32
          (local.get $mutexAddr) ;; mutex address
          (i32.const 1)          ;; expected value (1 => locked)
          (i64.const -1))        ;; infinite timeout

        ;; memory.atomic.wait32 returns:
        ;;   0 => "ok", woken by another agent.
        ;;   1 => "not-equal", loaded value != expected value
        ;;   2 => "timed-out", the timeout expired
        ;;
        ;; Since there is an infinite timeout, only 0 or 1 will be returned. In
        ;; either case we should try to acquire the mutex again, so we can
        ;; ignore the result.
        (drop)

        ;; Try to acquire the lock again.
        (br $retry)
      )
    )
  )

  ;; Unlock a mutex at the given address.
  (func $unlockMutex32
    (param $mutexAddr i32)
    ;; Unlock the mutex.
    (i32.atomic.store
      (local.get $mutexAddr)     ;; mutex address
      (i32.const 0))             ;; 0 => unlocked

    ;; Notify one agent that is waiting on this lock.
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)   ;; mutex address
        (i32.const 1)))          ;; notify 1 waiter
  )

  (func (export "run32")
    (call $lockMutex32 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex32 (i32.const 0))
  )

  ;; Below functions are the same as above with different integer sizes so
  ;; have comments elided see above to understand logic.

  (func $tryLockMutex64
    (param $mutexAddr i32) (result i32)
    (i64.atomic.rmw.cmpxchg
      (local.get $mutexAddr)
      (i64.const 0)
      (i64.const 1))
    (i64.eqz)
  )
  (func $lockMutex64
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        (call $tryLockMutex64 (local.get $mutexAddr))
        (br_if $done)
        (memory.atomic.wait64
          (local.get $mutexAddr)
          (i64.const 1)
          (i64.const -1))
        (drop)
        (br $retry)
      )
    )
  )
  (func $unlockMutex64
    (param $mutexAddr i32)
    (i64.atomic.store
      (local.get $mutexAddr)
      (i64.const 0))
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)
        (i32.const 1)))
  )
  (func (export "run64")
    (call $lockMutex64 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex64 (i32.const 0))
  )

  (func $tryLockMutex32_8
    (param $mutexAddr i32) (result i32)
    (i32.atomic.rmw8.cmpxchg_u
      (local.get $mutexAddr)
      (i32.const 0)
      (i32.const 1))
    (i32.eqz)
  )
  (func $lockMutex32_8
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        (call $tryLockMutex32_8 (local.get $mutexAddr))
        (br_if $done)
        (memory.atomic.wait32
          (local.get $mutexAddr)
          (i32.const 1)
          (i64.const -1))
        (drop)
        (br $retry)
      )
    )
  )
  (func $unlockMutex32_8
    (param $mutexAddr i32)
    (i32.atomic.store8
      (local.get $mutexAddr)
      (i32.const 0))
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)
        (i32.const 1)))
  )
  (func (export "run32_8")
    (call $lockMutex32_8 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex32_8 (i32.const 0))
  )

  (func $tryLockMutex32_16
    (param $mutexAddr i32) (result i32)
    (i32.atomic.rmw16.cmpxchg_u
      (local.get $mutexAddr)
      (i32.const 0)
      (i32.const 1))
    (i32.eqz)
  )
  (func $lockMutex32_16
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        (call $tryLockMutex32_16 (local.get $mutexAddr))
        (br_if $done)
        (memory.atomic.wait32
          (local.get $mutexAddr)
          (i32.const 1)
          (i64.const -1))
        (drop)
        (br $retry)
      )
    )
  )
  (func $unlockMutex32_16
    (param $mutexAddr i32)
    (i32.atomic.store16
      (local.get $mutexAddr)
      (i32.const 0))
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)
        (i32.const 1)))
  )
  (func (export "run32_16")
    (call $lockMutex32_16 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex32_16 (i32.const 0))
  )

  (func $tryLockMutex64_8
    (param $mutexAddr i32) (result i32)
    (i64.atomic.rmw8.cmpxchg_u
      (local.get $mutexAddr)
      (i64.const 0)
      (i64.const 1))
    (i64.eqz)
  )
  (func $lockMutex64_8
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        (call $tryLockMutex64_8 (local.get $mutexAddr))
        (br_if $done)
        (memory.atomic.wait64
          (local.get $mutexAddr)
          (i64.const 1)
          (i64.const -1))
        (drop)
        (br $retry)
      )
    )
  )
  (func $unlockMutex64_8
    (param $mutexAddr i32)
    (i64.atomic.store8
      (local.get $mutexAddr)
      (i64.const 0))
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)
        (i32.const 1)))
  )
  (func (export "run64_8")
    (call $lockMutex64_8 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex64_8 (i32.const 0))
  )

  (func $tryLockMutex64_16
    (param $mutexAddr i32) (result i32)
    (i64.atomic.rmw16.cmpxchg_u
      (local.get $mutexAddr)
      (i64.const 0)
      (i64.const 1))
    (i64.eqz)
  )
  (func $lockMutex64_16
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        (call $tryLockMutex64_16 (local.get $mutexAddr))
        (br_if $done)
        (memory.atomic.wait64
          (local.get $mutexAddr)
          (i64.const 1)
          (i64.const -1))
        (drop)
        (br $retry)
      )
    )
  )
  (func $unlockMutex64_16
    (param $mutexAddr i32)
    (i64.atomic.store16
      (local.get $mutexAddr)
      (i64.const 0))
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)
        (i32.const 1)))
  )
  (func (export "run64_16")
    (call $lockMutex64_16 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex64_16 (i32.const 0))
  )

  (func $tryLockMutex64_32
    (param $mutexAddr i32) (result i32)
    (i64.atomic.rmw32.cmpxchg_u
      (local.get $mutexAddr)
      (i64.const 0)
      (i64.const 1))
    (i64.eqz)
  )
  (func $lockMutex64_32
    (param $mutexAddr i32)
    (block $done
      (loop $retry
        (call $tryLockMutex64_32 (local.get $mutexAddr))
        (br_if $done)
        (memory.atomic.wait64
          (local.get $mutexAddr)
          (i64.const 1)
          (i64.const -1))
        (drop)
        (br $retry)
      )
    )
  )
  (func $unlockMutex64_32
    (param $mutexAddr i32)
    (i64.atomic.store32
      (local.get $mutexAddr)
      (i64.const 0))
    (drop
      (memory.atomic.notify
        (local.get $mutexAddr)
        (i32.const 1)))
  )
  (func (export "run64_32")
    (call $lockMutex64_32 (i32.const 0))
    (i32.store (i32.const 8) (i32.load (i32.const 8)) (i32.add (i32.const 1)))
    (call $unlockMutex64_32 (i32.const 0))
  )
)
