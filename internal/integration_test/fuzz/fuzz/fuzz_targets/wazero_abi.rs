//! This module provides the functions implemented by wazero via CGo.

extern "C" {
    // require_no_diff is implemented in Go, and accepts the pointer to the binary and its size.
    pub fn require_no_diff(
        binary_ptr: *const u8,
        binary_size: usize,
        wat_ptr: *const u8,
        wat_size: usize,
        check_memory: bool,
    );

    // validate is implemented in Go, and accepts the pointer to the binary and its size.
    pub fn validate(binary_ptr: *const u8, binary_size: usize);
}
