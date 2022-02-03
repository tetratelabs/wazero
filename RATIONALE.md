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


## Implementation limitations

[WebAssembly specification states that](https://www.w3.org/TR/wasm-core-1/#a2-implementation-limitations)
runtime implementors can impose their own limits on a number of aspects of a Wasm module or execution.

The followings are the limitations we explicitly impose and handle as errors during module instantiation in wazero:

### Number of functions in a store

The possible number of function instances in [a store](https://www.w3.org/TR/wasm-core-1/#store%E2%91%A0) is not specified in the WebAssembly specifications since [`funcaddr`](https://www.w3.org/TR/wasm-core-1/#syntax-funcaddr) corresponding to a function instance can be arbitrary number. 
In wazero, we choose to use `uint32` to represent `funcaddr`. Therefore the maximum number of function instances a store can instantiate is limited to 2^27. 

That is because not only we _believe_ that all use cases are fine with the limitation, but also we have no way to test wazero runtimes under these unusual circumstances.

### Number of function types in a store

There's no limitation on the number of function types in [a store](https://www.w3.org/TR/wasm-core-1/#store%E2%91%A0) according to the spec. In wazero implementation, we assign each function type to a unique ID, and choose to use `uint32` to represent the IDs.
Therefore the maximum number of function types a store can have is limited to 2^27. 

This is due to the same reason for the limitation on the number of functions above.

### Number of values on the stack in a function

According to the spec, there's no limitation on the number of values we a function can retain in the Wasm values stack. We limit the maximum number to 2^27 = 134,217,728.
The reason is that we internally represent all the values as 64-bit integes regardless of its types (including f32, f64), and 2^27 values means 
1 GiB = (2^30). 1 GiB is the reasonable for most applications [as we see a Goroutine has a stack size limit with that number on 64-bit arch](https://github.com/golang/go/blob/f296b7a6f045325a230f77e9bda1470b1270f817/src/runtime/proc.go#L120), considering that WebAssembly is (currently) 32-bit environment.

All the functions are statically analyzed at module insntantiation phase, and if a function can potentially reach this limit, an error is returend.

### Number of globals in a module

Theoretically, a module can declare globals (inclding imported ones) up to 2^32 times. However, we limit the number of available globals in a module to 2^27 = 134,217,728.
That is because internally we store globals in a slice with pointer types (meaning 8 bytes on 64-bit platforms), and thefore 2^27 globals
means that we have 1 GiB size of slice which seems large enough for most applications.

## JIT engine implementation

See [wasm/jit/RATIONALE.md](wasm/jit/RATIONALE.md).
