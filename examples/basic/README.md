## Basic example

This example shows how to extend a Go application with an addition function
defined in WebAssembly.

```bash
$ go run add.go 7 9
7 + 9 = 16
```

### Compilation

wazero is a WebAssembly runtime, embedded in your host application. To run
WebAssembly functions, you need access to a WebAssembly Binary (Wasm),
typically a `%.wasm` file.

[add.wasm](testdata/add.wasm) was compiled from [add.go](testdata/add.go) with
[TinyGo][1], as it is the most common way to compile Go source to Wasm. Here's
the minimal command to build a `%.wasm` binary.

```bash
(cd testdata; tinygo build -buildmode=c-shared -target=wasip1 -o add.wasm add.go)
```

### Notes

* Many other languages compile to (target) Wasm including AssemblyScript, C,
  C++, Rust, and Zig!
* The embedding application is often called the "host" in WebAssembly.
* The Wasm binary is often called the "guest" in WebAssembly. Sometimes they
  need [imports](../../imports) to implement features such as console output.
  TinyGo's `wasi` target, requires [WASI][2] imports.

[1]: https://wazero.io/languages/tinygo
[2]: https://wazero.io/specs/#wasi
