## wazero examples

The following example projects can help you practice WebAssembly with wazero:

* [allocation](allocation) - how to pass strings in and out of WebAssembly
  functions defined in Rust or TinyGo.
* [assemblyscript](../imports/assemblyscript/example) - how to configure
  special imports needed by AssemblyScript when not using WASI.
* [basic](basic) - how to use both WebAssembly and Go-defined functions.
* [import-go](import-go) - how to define, import and call a Go-defined function
  from a WebAssembly-defined function.
* [concurrent-instantiation](concurrent-instantiation) - how to instantiate multiple Wasm instances per Goroutine concurrently.
* [multiple-results](multiple-results) - how to return more than one result
  from WebAssembly or Go-defined functions.
* [multiple-runtimes](multiple-runtimes) - how to share compilation caches across multiple runtimes.
* [wasi](../imports/wasi_snapshot_preview1/example) - how to use I/O in your
  WebAssembly modules using WASI (WebAssembly System Interface).

Please [open an issue](https://github.com/tetratelabs/wazero/issues/new) if you
would like to see another example.
