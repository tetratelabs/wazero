## wazero imports

Packages in this directory implement the *host* imports needed for specific
languages or shared compiler toolchains.

* [AssemblyScript](assemblyscript) e.g. `asc X.ts --debug -b none -o X.wasm`
* [Emscripten](emscripten) e.g. `em++ ... -s STANDALONE_WASM -o X.wasm X.cc`
* [WASI](wasi_snapshot_preview1) e.g. `tinygo build -o X.wasm -target=wasi X.go`

Note: You may not see a language listed here because it either works without
host imports, or it uses WASI. Refer to https://wazero.io/languages/ for more.

Please [open an issue](https://github.com/tetratelabs/wazero/issues/new) if you
would like to see support for another compiled language or toolchain.

## Overview

WebAssembly has a virtual machine architecture where the *host* is the process
embedding wazero and the *guest* is a program compiled into the WebAssembly
Binary Format, also known as Wasm (`%.wasm`).

The only features that work by default are computational in nature, and the
only way to communicate is via functions, memory or global variables.

When a compiler targets Wasm, it often needs to import functions from the host
to satisfy system calls needed for functionality like printing to the console,
getting the time, or generating random values. The technical term for this
bridge is Application Binary Interface (ABI), but we'll call them simply host
imports.

Packages in this directory are sometimes well re-used, such as the case in
[WASI](https://wazero.io/specs/#wasi). For example, Rust, TinyGo, and Zig can
all target WebAssembly in a way that imports the same "wasi_snapshot_preview1"
module in the compiled `%.wasm` file. To support any of these, wazero users can
invoke `wasi_snapshot_preview1.Instantiate` on their `wazero.Runtime`.
