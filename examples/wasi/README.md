## WASI example

This example shows how to use I/O in your WebAssembly modules using WASI
(WebAssembly System Interface).

```bash
$ go run cat.go /test.txt
greet filesystem
```

If you do not set the environment variable `WASM_COMPILER`, main defaults
to use Wasm built with "tinygo". Here are the included examples:

* [tjnygo](testdata/tinygo) - Built via `tinygo build -o cat.wasm -scheduler=none --no-debug -target=wasi cat.go`
* [zig-cc](testdata/zig-cc) - Built via `zig cc cat.c -o cat.wasm --target=wasm32-wasi -O3`

Ex. To run the same example with zig-cc:
```bash
$ WASM_COMPILER=zig-cc go run cat.go /test.txt
greet filesystem
```

Note: While WASI attempts to be portable, there are no specification tests and
some compilers only partially implement features.

For example, Emscripten only supports WASI I/O on stdin/stdout/stderr, so
cannot be used for this example. See emscripten-core/emscripten#17167
