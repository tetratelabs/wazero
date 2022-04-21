## Allocation examples

The examples in this directory deal with memory allocation concerns in
WebAssembly, e.g. How to pass strings in and out of WebAssembly functions.

```bash
$ go run greet.go wazero
wasm >> Hello, wazero!
go >> Hello, wazero!
```

While the below examples use strings, they are written in a way that would work
for binary serialization.

* [Rust](rust) - Calls Wasm built with `cargo build --release --target wasm32-unknown-unknown`
* [TinyGo](tinygo) - Calls Wasm built with `tinygo build -o X.wasm -scheduler=none --no-debug -target=wasi X.go`

Note: The above are language-specific because there's no WebAssembly
specification for memory allocation. This mean exported functions are different
and how pointers map to parameters are different. That said, the examples are
as close to the same as possible with subtle differences and gotchas mentioned
in the respective README files.
