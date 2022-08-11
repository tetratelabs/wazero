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
* [Zig](zig) - Calls Wasm built with `zig build`

Note: Each of the above languages differ in both terms of exports and runtime
behavior around allocation, because there is no WebAssembly specification for
it. For example, TinyGo exports allocation functions while Rust and Zig don't.
Also, Rust eagerly collects memory before returning from a Wasm function while TinyGo
does not.

We still try to keep the examples as close to the same as possible, and
highlight things to be aware of in the respective source and README files.
