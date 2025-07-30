#![no_main]

use arbitrary::Arbitrary;
use libfuzzer_sys::arbitrary::{Result, Unstructured};
use libfuzzer_sys::fuzz_target;
use wasm_smith::Config;

mod util;

fuzz_target!(|data: &[u8]| {
    let _ = run(data);
});

fn run(data: &[u8]) -> Result<()> {
    // Create the random source.
    let mut u = Unstructured::new(data);

    // Generate the configuration with possibly invalid functions.
    let mut config = Config::arbitrary(&mut u)?;
    config.threads_enabled = true;
    config.tail_call_enabled = true;
    config.allow_invalid_funcs = true;

    let module = wasm_smith::Module::new(config.clone(), &mut u)?;
    let module_bytes = module.to_bytes();

    unsafe {
        util::validate(module_bytes.as_ptr(), module_bytes.len());
    }

    // We always return Ok as inside validate, we cause panic if the binary is interesting.
    Ok(())
}
