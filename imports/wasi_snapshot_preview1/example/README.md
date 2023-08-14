## WASI example

This example shows how to use I/O in your WebAssembly modules using WASI
(WebAssembly System Interface).

```bash
$ go run cat.go /test.txt
greet filesystem
```

If you do not set the environment variable `TOOLCHAIN`, main defaults
to use Wasm built with "tinygo". Here are the included examples:

* [cargo-wasi](testdata/cargo-wasi) - Built via `cargo wasi build --release; mv ./target/wasm32-wasi/release/cat.wasm .`
* [tinygo](testdata/tinygo) - Built via `tinygo build -o cat.wasm -scheduler=none --no-debug -target=wasi cat.go`
* [zig](testdata/zig) - Built via `zig build-exe cat.zig -target=wasm32-wasi -OReleaseSmall`
* [zig-cc](testdata/zig-cc) - Built via `zig cc cat.c -o cat.wasm --target=wasm32-wasi -O3`

To run the same example with zig-cc:

```bash
$ TOOLCHAIN=zig-cc go run cat.go /test.txt
greet filesystem
```

### Toolchain notes

Examples here check in the resulting binary, as doing so removes a potentially
expensive toolchain setup. This means we have to be careful how wasm is built,
so that the repository doesn't bloat (ex more bandwidth for `git clone`).

While WASI attempts to be portable, there are no specification tests and
some compilers partially implement features. Notes about portability follow.

### cargo-wasi (Rustlang)

The [Rustlang source](testdata/cargo-wasi) uses [cargo-wasi][1] because the
normal release target `wasm32-wasi` results in almost 2MB, which is too large
to check into our source tree.

Concretely, if you use `cargo build --target wasm32-wasi --release`, the binary
`./target/wasm32-wasi/release/cat.wasm` is over 1.9MB. One explanation for this
bloat is [`wasm32-wasi` target is not pure rust][2]. As that issue is over two
years old, it is unlikely to change. This means the only way to create smaller
wasm is via optimization.

The [cargo-wasi][3] crate includes many optimizations in its release target,
including `wasm-opt`, a part of [binaryen][4]. `cargo wasi build --release`
generates 82KB of wasm, which is small enough to check in.

### emscripten

Emscripten is not included as we cannot create a cat program without using
custom filesystem code. Emscripten only supports WASI I/O for
stdin/stdout/stderr and [suggest using wasi-libc instead][5]. This is used in
the [zig-cc](testdata/zig-cc) example.

[1]: https://github.com/bytecodealliance/cargo-wasi

[2]: https://github.com/rust-lang/rust/issues/73432

[3]: https://github.com/bytecodealliance/cargo-wasi

[4]: https://github.com/WebAssembly/binaryen

[5]: https://github.com/emscripten-core/emscripten/issues/17167#issuecomment-1150252755
