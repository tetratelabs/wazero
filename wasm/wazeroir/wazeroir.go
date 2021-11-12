// A pkg to compile down the standard Wasm binary to wazero's specific IR (wazeroir).
// The wazeroir is inspired by microwasm format (a.k.a. LightbeamIR), previously used
// in the lightbeam JIT compiler in Wasmtime, though it is not specified and only exists
// in the previous codebase of wasmtime
// e.g. https://github.com/bytecodealliance/wasmtime/blob/v0.29.0/crates/lightbeam/src/microwasm.rs
//
// The main difference from microwasm is that it doesn't have block operations
// in order to simplify the implementation.
// This package also has an implementation of direct interpreter of wazeroir which
// implements the wasm.Engine interface, meaning that it can be used as an engine
// of wazero runtime.
package wazeroir
