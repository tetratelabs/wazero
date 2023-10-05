//! This module provides the functions implemented by wazero via CGo.

use wasm_smith::SwarmConfig;

extern "C" {
    // require_no_diff is implemented in Go, and accepts the pointer to the binary and its size.
    #[allow(dead_code)]
    pub fn require_no_diff(
        binary_ptr: *const u8,
        binary_size: usize,
        wat_ptr: *const u8,
        wat_size: usize,
        check_memory: bool,
    );

    // validate is implemented in Go, and accepts the pointer to the binary and its size.
    #[allow(dead_code)]
    pub fn validate(binary_ptr: *const u8, binary_size: usize);
}

#[allow(dead_code)]
pub fn maybe_disable_simd(config: &mut SwarmConfig) {
    if std::env::var("WAZERO_FUZZ_WAZEVO").is_ok() {
        // Wazevo doesn't support SIMD yet.
        config.simd_enabled = false;
    }
}
