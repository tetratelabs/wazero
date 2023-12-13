extern crate libc;
extern crate nix;

use libc::SIGSTKSZ;
use libc::{pthread_kill, pthread_self, SIGUSR1};
use nix::sys::signal;
use nix::sys::signal::{sigaction, SaFlags, SigAction, SigHandler, SigSet};
use std::thread;
use std::time::Duration;

const STACK_SIZE: usize = SIGSTKSZ * 4;

#[test]
fn main() {
    unsafe {
        let sa = SigAction::new(
            SigHandler::SigAction(handler),
            // Set SA_ONSTACK to ensure the signal handler runs on the alternate stack.
            // The alternate stack is prepared by the Go runtime if there's not the one by the host C program
            // via the sigaltstack syscall. However if this flag is not set, which happens when
            // the host C program not having intention to deal with signals gracefully (e.g. stack overflow),
            // the signal handler (installed by either C program or Go runtime) will run on
            // the "current stack". That is problematic when a signal handling happens during execution
            // of wazevo function because it uses "Go allocated" stack.
            //
            // On the other hand, this is more of a general problem for any C program that uses Go as a library,
            // not limited to wazevo, when it does not not install sig handlers with SA_ONSTACK. That means
            // any Gorountime stack could result in being used during signal handling, which can potentially
            // cause any memory corruption. I would say such C program is using Go library in a dangerous way.
            //
            // To reproduce the failure in wazevo, Use SaFlags::empty() and wazevoapi.StackGuardCheckEnabled=true.
            //
            // Note that this only happens a Go program is compiled as c-archive or c-shared. If it is
            // used normally, the signal handlers are installed on each signal by the Go runtime, which
            // sets SA_ONSTACK and proper alternate stack, hence there's no way the current stack is used
            // during singal handling.
            SaFlags::SA_ONSTACK,
            SigSet::empty(),
        );

        if let Err(err) = sigaction(signal::SIGUSR1, &sa) {
            panic!("Failed to set signal handler: {}", err);
        }

        let main_thread_id = pthread_self();
        thread::spawn(move || loop {
            thread::sleep(Duration::from_millis(1));
            pthread_kill(main_thread_id, SIGUSR1);
        });
        test_signal_stack();
    }
}

extern "C" fn handler(_: libc::c_int, _: *mut libc::siginfo_t, _: *mut libc::c_void) {
    // Declare a large local array to use the stack space.
    let mut large_array: [u8; 1024] = [0; 1024];

    // Use the array to prevent compiler optimizations from removing it.
    for i in 0..large_array.len() {
        large_array[i] = i as u8;
    }
    if large_array[100] != 100 {
        panic!("large_array[0] != 0");
    }
}

extern "C" {
    pub fn test_signal_stack();
}
