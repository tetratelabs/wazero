## Rust allocation example

This example shows how to pass strings in and out of a Wasm function defined
in Rust, built with `cargo build --release --target wasm32-unknown-unknown`

```bash
$ go run greet.go wazero
Hello, wazero!
```

Under the covers, [lib.rs](testdata/src/lib.rs) does a few things of interest:
* Uses a WebAssembly-tuned memory allocator: [wee_alloc](https://github.com/rustwasm/wee_alloc).
* Exports wrapper functions to allocate and deallocate memory.
* Uses `&str` instead of CString (NUL-terminated strings).
* Uses `std::mem::forget` to prevent Rust from eagerly freeing pointers returned.

Note: We chose to not use CString because it keeps the example similar to how
you would track memory for arbitrary blobs. We also watched function signatures
carefully as Rust compiles different WebAssembly signatures depending on the
input type.

See https://wazero.io/languages/rust/ for more tips.
