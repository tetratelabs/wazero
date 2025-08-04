# Notable rationale of wazero (Engine)

## Tail Call Implementation

The tail call implementation in wazero takes some liberties at interpreting the WebAssembly spec, with the goal of correctness, if not strict adherence to the spec. In principle

- in the interpreter, tail calls are implemented by simply resetting the program counter to the start of the function being called
- in the compiler, tail calls are implemented by emitting a jump instruction to the start of the function.

These general rules are however subject to some exceptions, which are detailed below.

### Interpreter

There are 4 cases that needs to be handled in the interpreter:

1. **Tail calls to the same function**: this is the simplest case, where we just reset the program counter to the start of the function body. This is straightforward and does not require any special handling.
2. **Tail calls to a different function**: this is also straightforward, as we just reset the program counter to the start of the function body, then replace the function body with the new function's body. 
3. **Tail calls to an imported function**: in this case, we cannot simply reset the program counter, as the target function is not defined as part of the current WebAssembly module: jumping into the imported module would require a lot more book-keeping. For simplicity, we fall back to a plain call to the imported function, which is the same as if we were calling it from a regular function.
4. **Tail calls to a host function**: similarly to imported funcions, host functions are defined externally to the current WebAssembly module; moreover they are not defined in WebAssembly, but in the host language, making the straightforward strategy we used above impossible; in this case we also fall back to a plain call.

### Compiler 

Consistently with the WebAssembly spec, which states that a tail call behaves ["like a combination of `return` followed by a respective call](https://github.com/WebAssembly/tail-call/blob/main/proposals/tail-call/Overview.md#execution), 
the compiler for both backends restores the stack as it would normally do in the epilogue (in fact, it invokes `setupEpilogueAfter()` in `postRegAlloc()`), then it jumps to the beginning of the function. 

In particular, on `amd64`, indirect calls also move the function pointer to a safe (caller-saved) register (`r11`) to make sure it will not be overwritten in the epilogue.

Similarly to the interpreter, in some cases the compiler will fall back to a plain call instead of a tail call. The rule of thumb is that _the compiler will handle tail calls as long as they are "compatible" with the calling convention of the target architecture_. In particular, both `amd64` and `arm64` will use register arguments for a certain number of arguments (depending on the architecture and the type of the arguments), above this threshold, arguments are passed on the stack. The compiler will handle the tail call as long as _no stack arguments are involved_; the reasons are:

  a. if the sizes of the required stack space do not match, then we might be writing data at a wrong address
  b. even if the sizes match, we would need special attention not to clear the stack completely before we jump into the callee
  c. even if we managed to do that, the spill slots are not necessarily matching so, we would still need some special care (notice that other runtimes have completely revolutionized their internal calling conventions _for all calls_ to accommodate tail calls)

Because this is an architecture-specific limitation, the front-end **always** emits `Return` instructions together with the tail call instructions, and the back-end is responsible for deciding whether to emit a tail call or fall back to a regular call:
      
      - if the tail call **cannot** be handled (i.e., there are stack arguments) the tail call is essentially interpreted as a synonym for a plain call, and the return handling code is kept; 
      - if the tail call CAN be handled, THEN we just remove the useless instructions between the tail-call and the ret instruction during Finalize (postRegAlloc()), as it's dead code anyway.

Finally, in the compiler, indirect calls to local functions, calls host functions and calls to imported functions are all implemented using function pointers. As opposed to the interpreter, they do not require any special handling, but they will still fall back to a plain call if the tail call cannot be handled (i.e., there are stack arguments).
