#![no_main]

use libfuzzer_sys::arbitrary::{Result, Unstructured};
use libfuzzer_sys::fuzz_target;

fuzz_target!(|data: &[u8]| {
    drop(run(data));
});

fn run(data: &[u8]) -> Result<()> {
    // Create the random source.
    let mut u = Unstructured::new(data);

    // Generate the random module via wasm-smith, but MaybeInvalidModule.
    let module: wasm_smith::MaybeInvalidModule = u.arbitrary()?;
    let module_bytes = module.to_bytes();
    let wat_bytes = wasmprinter::print_bytes(&module_bytes).unwrap();

    // Pass the randomly generated module to the wazero library.
    unsafe {
        validate(
            module_bytes.as_ptr(),
            module_bytes.len(),
            wat_bytes.as_ptr(),
            wat_bytes.len(),
        );
    }

    // We always return Ok as inside require_no_diff, we cause panic if the binary is interesting.
    Ok(())
}

extern "C" {
    // validate is implemented in Go, and accepts the pointer to the binary and its size.
    fn validate(binary_ptr: *const u8, binary_size: usize, wat_ptr: *const u8, wat_size: usize);
}
