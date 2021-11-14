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
