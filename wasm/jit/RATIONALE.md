# Just-In-Time compilation engine

This package implements the JIT engine for WebAssembly *purely written in Go*. 
In this README, we describe the background, technical difficulties and some of the design choices.

## General limitations on pure Go JIT engines

In Go program, each Goroutine manages its own stack, and each item on Goroutine stack is managed by Go runtime for garbage collection, etc.

These impose some difficulties on JIT engine purely written in Go because we *cannot* use native push/pop instructions to save/restore temporaly variables spilling from registers. This results in making it impossible for us to invoke Go functions from JITed native codes with the native `call` instruction since it involves stack manipulations.

*TODO: maybe it is possible to hack the runtime to make it possible to achieve function calls with `call`.*

## How to generate native codes

Currently we rely on [`twitchyliquid64/golang-asm`](https://github.com/twitchyliquid64/golang-asm) to assemble native codes. The library is just a copy of Go official compiler's assembler with modified import paths. So once we reach some maturity, we could implement our own assembler to reduce the unnecessary dependency as being less dependency is one of our primary goal in this project.

The assembled native codes are represented as `[]byte` and the slice region is marked as executable via mmap system call.

## How to enter native codes

Assuming that we have a native code as `[]byte`, it is straightforward to enter the native code region via 
Go assembly code. In this package, we have the function without body called `jitcall`

```go
func jitcall(codeSegment, engine, memory uintptr)
```

where we pass `codeSegment uintptr` as a first argument. This pointer is pointing to the first instruction to be executed. The pointer can be easily derived from `[]byte` via `unsafe.Pointer`:
```go
code := []byte{}
/* ...Compilation ...*/
codeSegment := uintptr(unsafe.Pointer(&code[0]))
jitcall(codeSegment, ...)
```

And `jitcall` is actually implemented in [jit_amd64.s](./jit_amd64.s) as a convenience layer to comply with the Go's official calling convention and we delegate the task to jump into the code segment to the Go assembler code.

## How to achieve function calls

Given that we cannot use `call` instruction at all in native code, here's how we achieve the function calls back and forth among Go and (JITed) Wasm native functions.

The general principle is that all the function calls consists of 1) emitting instruction to record the continuation program counter to `engine.continuationAddressOffset` 2) emitting `return` instruction.

For example, the following Wasm code

```
0x3: call 1
0x5: i64.const 100
```

will be compiled as 

```
mov [engine.functionCallIndex] $1 ;; Set the index of call target function to functionCallIndex field of engine.
mov [engine.continuationAddressOffset] $0x05 ;; Set the continuation address to continuationAddressOffset field of engine.
return ;; Return from the function.
mov ... $100 ;; This is the beginning of program *after* function return.
```

This way, the engine, which enters the native code via `jitcall`, can know the continuation address of the caller's function frame: 

```go
case jitCallStatusCodeCallWasmFunction:
    nextFunc := e.compiledWasmFunctions[e.functionCallIndex]
    // Calculate the continuation address so
    // we can resume this caller function frame.
    currentFrame.continuationAddress = currentFrame.f.codeInitialAddress + e.continuationAddressOffset
    currentFrame.continuationStackPointer = e.stackPointer + nextFunc.outputNum - nextFunc.inputNum
    currentFrame.stackBasePointer = e.stackBasePointer
```

and calling into another function in JIT engine's main loop:

```go
    // Create the callee frame.
    frame := &callFrame{
        continuationAddress: nextFunc.codeInitialAddress,
        f:                   nextFunc,
        // Set the caller frame so we can return back to the current frame!
        caller: currentFrame,
        // Set the base pointer to the beginning of the function inputs
        stackBasePointer: e.stackBasePointer + e.stackPointer - nextFunc.inputNum,
    }
```

After finished executing the callee code, we return back to the caller's code with the specified return address:

```go
case jitStatusReturned:
    // Meaning that the current frame exits
    // so we just get back to the caller's frame.
    callerFrame := currentFrame.caller
    e.callFrameStack = callerFrame
    e.stackBasePointer = callerFrame.stackBasePointer
    e.stackPointer = callerFrame.continuationStackPointer
```

To summarize, every function call is achieved by returning back to Go code (`engine.exec`'s main loop) with some continuation infor, and enter the callee native code (or host functions) from there. That, of course, comes with a bit of overhead because each function call is implemented by two steps (returning back to `jitcall` callsite AND entering `jitcall` again) vs just `call` instruction (or `jmp`) in usual native codes.

Note that this mechanism is a minimal PoC impl, so in the near future, we would achieve the function calls without returning back to `engine.exec`'s main loop and instead `jmp` directly to the callee native code.
