use std::env::var;
use std::process;
use std::str;

fn main() {
    // Get the absolute path to the root of this repository (not cargo target).
    let wazero_fuzz_dir = format!("{}/..", var("CARGO_MANIFEST_DIR").unwrap());

    let wazero_fuzz_lib_dir = format!("{}/wazerolib", wazero_fuzz_dir.as_str());
    let library_out_path = format!("{}/libwazero.a", wazero_fuzz_lib_dir);
    let library_source_path = format!("{}/...", wazero_fuzz_lib_dir);

    // Parse the GOARCH from the --target argument passed to cargo.
    let goarch = var("TARGET")
        .map(|target| {
            if target.contains("aarch64") {
                "arm64"
            } else if target.contains("x86") {
                "amd64"
            } else {
                panic!("unsupported target {:?}", target)
            }
        })
        .unwrap();

    // Build the wazero library via go build -buildmode c-archive....
    let mut command = process::Command::new("go");
    command.current_dir(&wazero_fuzz_lib_dir);
    command.arg("build");
    command.args(["-buildmode", "c-archive"]);
    command.args(["-o", library_out_path.as_str()]);
    command.args([library_source_path.as_str()]);
    command.env("GOARCH", goarch);
    command.env("CGO_ENABLED", "1");

    let output = command.output().expect("failed to execute process");

    // If the build didn't succeed, exit the process with the stderr from Go's command.
    if !output.status.success() {
        panic!(
            "failed to compile wazero lib: {}\n",
            str::from_utf8(&output.stderr).unwrap(),
        );
    }

    // Ensures that we rebuild the library when the source code for wazero file has been changed.
    println!("cargo:rerun-if-changed={}/../../..", wazero_fuzz_dir);

    // Ensures that the linker can find the wazero library.
    println!("cargo:rustc-link-search={}", wazero_fuzz_lib_dir);
    println!("cargo:rustc-link-lib=static=wazero");
}
