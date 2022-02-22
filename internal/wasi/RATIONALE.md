# Notable rationale of the WASI implementation in wazero

## Semantics of WASI API

Unfortunately, (WASI Snapshot Preview 1)[https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md] is not formally defined enough, and has APIs with ambiguous semantics.
This section describes how Wazero interprets and implements the semantics of several WASI APIs that may be interpreted differently by different wasm runtimes.
Those APIs may affect the portability of a WASI application.

### FdPrestatDirName

`FdPrestatDirName` is a WASI function to return the path of the pre-opened directory of a file descriptor.
It has the following three parameters, and the third `pathLen` has ambiguous semantics.

- `fd` - a file descriptor
- `path` - the offset for the result path
- `pathLen` - In wazero, `FdPrestatDirName` writes the result path string to `path` offset for the exact length of `pathLen`.

Wasmer considers `pathLen` to be the maximum length instead of the exact length that should be written.
See https://github.com/wasmerio/wasmer/blob/3463c51268ed551933392a4063bd4f8e7498b0f6/lib/wasi/src/syscalls/mod.rs#L764

The semantics in wazero follows that of wasmtime.
See https://github.com/bytecodealliance/wasmtime/blob/2ca01ae9478f199337cf743a6ab543e8c3f3b238/crates/wasi-common/src/snapshots/preview_1.rs#L578-L582

Their semantics match when `pathLen` == the length of `path`, so in practice this difference won't matter match.
