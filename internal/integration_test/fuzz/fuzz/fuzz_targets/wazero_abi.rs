//! This module provides the functions implemented by wazero via CGo.

extern "C" {
    // require_no_diff is implemented in Go, and accepts the pointer to the binary and its size.
    #[allow(dead_code)]
    pub fn require_no_diff(binary_ptr: *const u8, binary_size: usize, check_memory: bool);

    // validate is implemented in Go, and accepts the pointer to the binary and its size.
    #[allow(dead_code)]
    pub fn validate(binary_ptr: *const u8, binary_size: usize);
}

use ctor::ctor;
use libc::SIGSTKSZ;
use nix::libc::{sigaltstack, stack_t};
use nix::sys::signal::{sigaction, SaFlags, SigAction, SigHandler, SigSet, Signal};
use std::ptr::null_mut;

#[ctor]
/// Sets up the separate stack for signal handlers, and sets the SA_ONSTACK flag for signals that are handled by libFuzzer
///  https://github.com/llvm/llvm-project/blob/8eff5704829ba5edd28754fd9ec7665b34fde22a/compiler-rt/lib/fuzzer/FuzzerUtilPosix.cpp#L117-L141
/// in order to ensure that Go's stacks won't get corrupted accidentally.
///
/// This is necessary due to the undocumented requirement/behavior of Go runtime, and for detail,
/// see the detailed comments in `tests/sigstack.rs`.
fn setup_sig_handlers() {
    set_signal_stack();
    set_sa_on_stack(Signal::SIGABRT);
    set_sa_on_stack(Signal::SIGALRM);
    set_sa_on_stack(Signal::SIGBUS);
    set_sa_on_stack(Signal::SIGFPE);
    set_sa_on_stack(Signal::SIGILL);
    set_sa_on_stack(Signal::SIGINT);
    set_sa_on_stack(Signal::SIGSEGV);
    set_sa_on_stack(Signal::SIGTERM);
    set_sa_on_stack(Signal::SIGXFSZ);
    set_sa_on_stack(Signal::SIGUSR1);
    set_sa_on_stack(Signal::SIGUSR2);
}

/// Sets the SA_ONSTACK flag for the given signal.
fn set_sa_on_stack(sig: Signal) {
    let old_action = unsafe {
        let tmp = SigAction::new(SigHandler::SigDfl, SaFlags::empty(), SigSet::empty());
        sigaction(sig, &tmp).unwrap()
    };
    // Create a new SigAction with the SA_ONSTACK flag added.
    let new_flags = old_action.flags() | SaFlags::SA_ONSTACK;
    let new_action = SigAction::new(old_action.handler(), new_flags, old_action.mask());
    unsafe {
        sigaction(sig, &new_action).unwrap();
    }
}

/// Sets up the separate stack for signal handlers.
fn set_signal_stack() {
    // Allocate a new stack for signal handlers to run on.
    const STACK_SIZE: usize = SIGSTKSZ * 2;
    let mut stack = vec![0u8; STACK_SIZE];

    let stack_ptr = stack.as_mut_ptr();

    let signal_stack = stack_t {
        ss_sp: stack_ptr as *mut libc::c_void,
        ss_flags: 0,
        ss_size: STACK_SIZE,
    };

    unsafe {
        if sigaltstack(&signal_stack, null_mut()) != 0 {
            panic!("Failed to set alternate signal stack");
        }

        // Leak the stack vector to prevent it from being dropped.
        std::mem::forget(stack);
    }
}
