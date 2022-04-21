extern crate alloc;
extern crate core;
extern crate wee_alloc;

use alloc::vec::Vec;
use std::mem::MaybeUninit;
use std::slice;

/// Prints a greeting to the console using [`log`].
fn greet(name: &String) {
    log(&["Greet, ", &name, "!"].concat());
}

/// gets a greeting for the name.
fn greeting(name: &String) -> &String {
    return &["Greet, ", &name, "!"].concat();
}

/// Logs a message to the console using [`_log`].
fn log(message: &String) {
    unsafe {
        _log(message.as_ptr(), message.len());
    }
}

#[link(wasm_import_module = "env")]
extern "C" {
    /// WebAssembly import which prints a string (linear memory offset,
    /// byteCount) to the console.
    ///
    /// Note: This is not an ownership transfer: Rust still owns the pointer
    /// and ensures it isn't deallocated during this call.
    #[link_name = "log"]
    fn _log(ptr: *const u8, size: usize);
}

/// WebAssembly export that accepts a string (linear memory offset, byteCount)
/// and calls [`greet`].
///
/// Note: The input parameters were returned by [`allocate`]. This is not an
/// ownership transfer, so the inputs can be reused after this call.
#[cfg_attr(
all(target_arch = "wasm32", target_os = "unknown"),
export_name = "greet"
)]
#[no_mangle]
pub unsafe extern "C" fn _greet(ptr: *mut u8, size: usize) {
    // Borrow
    let slice = slice::from_raw_parts_mut(ptr, size);
    let name = std::str::from_utf8_unchecked_mut(slice);
    greet(&String::from(name));
}

/// Set the global allocator to the WebAssembly optimized one.
#[global_allocator]
static ALLOC: wee_alloc::WeeAlloc = wee_alloc::WeeAlloc::INIT;

/// WebAssembly export that allocates a pointer (linear memory offset) that can
/// be used for a string.
///
/// This is an ownership transfer, which means the caller must call
/// [`deallocate`] when finished.
#[cfg_attr(
all(target_arch = "wasm32", target_os = "unknown"),
export_name = "allocate"
)]
#[no_mangle]
pub extern "C" fn allocate(size: usize) -> *mut u8 {
    // Allocate the amount of bytes needed.
    let vec: Vec<MaybeUninit<u8>> = Vec::with_capacity(size);

    // into_raw leaks the memory to the caller.
    Box::into_raw(vec.into_boxed_slice()) as *mut u8
}


/// WebAssembly export that deallocates a pointer of the given size (linear
/// memory offset, byteCount) allocated by [`allocate`].
#[cfg_attr(
all(target_arch = "wasm32", target_os = "unknown"),
export_name = "deallocate"
)]
#[no_mangle]
pub extern "C" fn deallocate(ptr: *mut u8, size: usize) {
    unsafe {
        // Retake the pointer which allows its memory to be freed.
        let _ = Vec::from_raw_parts(ptr, 0, size);
    }
}
