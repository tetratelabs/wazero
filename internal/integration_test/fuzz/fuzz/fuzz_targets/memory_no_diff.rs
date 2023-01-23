//! memory_diff fuzzing test is almost the same as basic.rs
//! but this also ensure that there's no difference in the memory buffer
//! state after the execution.

#![no_main]
use libfuzzer_sys::arbitrary::{Result, Unstructured};
use libfuzzer_sys::fuzz_target;
use wasm_smith::SwarmConfig;
mod wazero_abi;

fuzz_target!(|data: &[u8]| {
    drop(run(data));
});

fn run(data: &[u8]) -> Result<()> {
    // Create the random source.
    let mut u = Unstructured::new(data);

    // Generate the configuration.
    let mut config: SwarmConfig = u.arbitrary()?;

    // 64-bit memory won't be supported by wazero.
    config.memory64_enabled = false;
    // For exactly one memory exists.
    config.max_memories = 1;
    config.min_memories = 1;
    // If we don't set the limit, we will soon reach the OOM and the fuzzing will be killed by OS.
    config.max_memory_pages = 10;
    config.memory_max_size_required = true;
    // Don't test too large tables.
    config.max_tables = 2;
    config.max_table_elements = 1_000;
    config.table_max_size_required = true;

    // max_instructions is set to 100 by default which seems a little bit smaller.
    config.max_instructions = 5000;

    // Without canonicalization of NaNs, the results cannot be matched among engines.
    config.canonicalize_nans = true;

    // Export all the things so that we can invoke them.
    config.export_everything = true;

    // Ensures that at least one function exists.
    config.min_funcs = 1;
    config.max_funcs = config.max_funcs.max(1);

    // Generate the random module via wasm-smith.
    let mut module = wasm_smith::Module::new(config.clone(), &mut u)?;
    module.ensure_termination(1000);
    let module_bytes = module.to_bytes();

    let wat_bytes = wasmprinter::print_bytes(&module_bytes).unwrap();

    // Pass the randomly generated module to the wazero library.
    unsafe {
        wazero_abi::require_no_diff(
            module_bytes.as_ptr(),
            module_bytes.len(),
            wat_bytes.as_ptr(),
            wat_bytes.len(),
            true,
        );
    }

    // We always return Ok as inside require_no_diff, we cause panic if the binary is interesting.
    Ok(())
}
