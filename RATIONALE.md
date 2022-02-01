# Notable rationale of wazero

## wazeroir
wazero's intermediate representation (IR) is called `wazeroir`. Compiling into an IR provides us a faster interpreter
and a building block for a future JIT compilation engine. Both of these help answer demands for a more performant
runtime vs interpreting Wasm directly (the `naivevm` interpreter).

### Intermediate Representation (IR) design
`wazeroir`'s initial design borrowed heavily from the defunct `microwasm` format (a.k.a. LightbeamIR). Notably,
`wazeroir` doesn't have block operations: this simplifies the implementation.

Note: `microwasm` was never specified formally, and only exists in a historical codebase of wasmtime:
https://github.com/bytecodealliance/wasmtime/blob/v0.29.0/crates/lightbeam/src/microwasm.rs


## Size limitations
### Number of functions

The possible number of function instances in [a store](https://www.w3.org/TR/wasm-core-1/#store%E2%91%A0) is not specified in the WebAssembly specifications since [`funcaddr`](https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr) corresponding to a function instance can be arbitrary number. 
In wazero, we choose to use `uint32` to represent `funcaddr`. Therefore the maximum number of function instances a store can instantiate is limited to 2^32. 

That is because not only we _believe_ that all use cases are fine with the limitation, but also we have no way to test wazero runtimes under these unusual circumstances.

### Number of function types

There's no limitation on the number of function types in [a store](https://www.w3.org/TR/wasm-core-1/#store%E2%91%A0) according to the spec. In wazero implementation, we assign each function type to a unique ID, and choose to use `uint32` to represent the IDs.
Therefore the maximum number of function types a store can have is limited to 2^32. 

This is due to the same reason for the limitation on the number of functions above.

## JIT engine implementation

See [wasm/jit/RATIONALE.md](wasm/jit/RATIONALE.md).
