#![no_main]

use libfuzzer_sys::arbitrary::{Result, Unstructured};
use libfuzzer_sys::fuzz_target;

mod wazero_abi;

fuzz_target!(|data: &[u8]| {
    drop(run(data));
});

fn run(data: &[u8]) -> Result<()> {
    // Create the random source.
    let mut u = Unstructured::new(data);

    // Generate the random module via wasm-smith, but MaybeInvalidModule.
    let module: wasm_smith::MaybeInvalidModule = u.arbitrary()?;
    let module_bytes = module.to_bytes();

    unsafe {
        wazero_abi::validate(module_bytes.as_ptr(), module_bytes.len());
    }

    // We always return Ok as inside validate, we cause panic if the binary is interesting.
    Ok(())
}
