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

pub fn maybe_disable_v2(config: &mut SwarmConfig) {
    if std::env::var("WAZERO_FUZZ_WAZEVO").is_ok() {
        config.simd_enabled = false;
        config.multi_value_enabled = false;
        config.bulk_memory_enabled = false;
        config.reference_types_enabled = false;
        config.saturating_float_to_int_enabled = false;
        config.max_tables = 1;
    }
}
