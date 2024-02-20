#![no_main]
use libfuzzer_sys::fuzz_target;
mod util;

fuzz_target!(|data: &[u8]| {
    let _ = util::run_nodiff(data, false, false);
});
