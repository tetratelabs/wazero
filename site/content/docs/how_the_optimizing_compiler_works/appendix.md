+++
title = "Appendix: Trampolines"
layout = "single"
+++

Trampolines are used to interface between the Go runtime and the generated
code, in two cases:

- when we need to **enter the generated code** from the Go runtime.
- when we need to **leave the generated code** to invoke a host function
  (written in Go).

In this section we want to complete the picture of how a Wasm function gets
translated from Wasm to executable code in the optimizing compiler, by
describing how to jump into the execution of the generated code at run-time.

## Entering the Generated Code

At run-time, user space invokes a Wasm function through the public
`api.Function` interface, using methods `Call()` or `CallWithStack()`.  The
implementation of this method, in turn, eventually invokes an ASM
**trampoline**. The signature of this trampoline in Go code is:

```go
func entrypoint(
	preambleExecutable, functionExecutable *byte,
	executionContextPtr uintptr, moduleContextPtr *byte,
	paramResultStackPtr *uint64,
	goAllocatedStackSlicePtr uintptr)
```

- `preambleExecutable` is a pointer to the generated code for the preamble (see
  below)
- `functionExecutable` is a pointer to the generated code for the function (as
  described in the previous sections).
- `executionContextPtr` is a raw pointer to the `wazevo.executionContext`
  struct. This struct is used to save the state of the Go runtime before
entering or leaving the generated code. It also holds shared state between the
Go runtime and the generated code, such as the exit code that is used to
terminate execution on failure, or suspend it to invoke host functions.
- `moduleContextPtr` is a pointer to the `wazevo.moduleContextOpaque` struct.
  This struct Its contents are basically the pointers to the module instance,
specific objects as well as functions. This is sometimes called "VMContext" in
other Wasm runtimes.
- `paramResultStackPtr` is a pointer to the slice where the arguments and
  results of the function are passed.
- `goAllocatedStackSlicePtr` is an aligned pointer to the Go-allocated stack
  for holding values and call frames. For further details refer to
  [Backend § Prologue and Epilogue](../backend/#prologue-and-epilogue)

The trampoline can be found in`backend/isa/<arch>/abi_entry_<arch>.s`.

For each given architecture, the trampoline:
- moves the arguments to specific registers to match the behavior of the entry preamble or trampoline function, and
- finally, it jumps into the execution of the generated code for the preamble

The **preamble** that will be jumped from `entrypoint` function is generated per function signature.

This is implemented in `machine.CompileEntryPreamble(*ssa.Signature)`.

The preamble sets the fields in the `wazevo.executionContext`.

At the beginning of the preamble:

- Set a register to point to the `*wazevo.executionContext` struct.
- Save the stack pointers, frame pointers, return addresses, etc. to that
  struct.
- Update the stack pointer to point to `paramResultStackPtr`.

The generated code works in concert with the assumption that the preamble has
been entered through the aforementioned trampoline. Thus, it assumes that the
arguments can be found in some specific registers.

The preamble then assigns the arguments pointed at by `paramResultStackPtr` to
the registers and stack location that the generated code expects.

Finally, it invokes the generated code for the function.

The epilogue reverses part of the process, finally returning control to the
caller of the `entrypoint()` function, and the Go runtime. The caller of
`entrypoint()` is also responsible for completing the cleaning up procedure by
invoking `afterGoFunctionCallEntrypoint()` (again, implemented in
backend-specific ASM).  which will restore the stack pointers and return
control to the caller of the function.

The arch-specific code can be found in
`backend/isa/<arch>/abi_entry_preamble.go`.

[wazero-engine-stack]: https://github.com/tetratelabs/wazero/blob/095b49f74a5e36ce401b899a0c16de4eeb46c054/internal/engine/compiler/engine.go#L77-L132
[abi-arm64]: https://tip.golang.org/src/cmd/compile/abi-internal#arm64-architecture
[abi-amd64]: https://tip.golang.org/src/cmd/compile/abi-internal#amd64-architecture
[abi-cc]: https://tip.golang.org/src/cmd/compile/abi-internal#function-call-argument-and-result-passing


## Leaving the Generated Code

In "[How do compiler functions work?][how-do-compiler-functions-work]", we
already outlined how _leaving_ the generated code works with the help of a
function. We will complete here the picture by briefly describing the code that
is generated.

When the generated code needs to return control to the Go runtime, it inserts a
meta-instruction that is called `exitSequence` in both `amd64` and `arm64`
backends.  This meta-instruction sets the `exitCode` in the
`wazevo.executionContext` struct, restore the stack pointers and then returns
control to the caller of the `entrypoint()` function described above.

As described in "[How do compiler functions
work?][how-do-compiler-functions-work]", the mechanism is essentially the same
when invoking a host function or raising an error. However, when a function is
invoked the `exitCode` also indicates the identifier of the host function to be
invoked.

The magic really happens in the `backend.Machine.CompileGoFunctionTrampoline()`
method.  This method is actually invoked when host modules are being
instantiated.  It generates a trampoline that is used to invoke such functions
from the generated code.

This trampoline implements essentially the same prologue as the `entrypoint()`,
but it also reserves space for the arguments and results of the function to be
invoked.

A host function has the signature:

```
func(ctx context.Context, stack []uint64)
```

the function arguments in the `stack` parameter are copied over to the reserved
slots of the real stack. For instance, on `arm64` the stack layout would look
as follows (on `amd64` it would be similar):

```goat
                  (high address)
    SP ------> +-----------------+  <----+
               |     .......     |       |
               |      ret Y      |       |
               |     .......     |       |
               |      ret 0      |       |
               |      arg X      |       |  size_of_arg_ret
               |     .......     |       |
               |      arg 1      |       |
               |      arg 0      |  <----+ <-------- originalArg0Reg
               | size_of_arg_ret |
               |  ReturnAddress  |
               +-----------------+ <----+
               |      xxxx       |      |  ;; might be padded to make it 16-byte aligned.
          +--->|  arg[N]/ret[M]  |      |
 sliceSize|    |   ............  |      | goCallStackSize
          |    |  arg[1]/ret[1]  |      |
          +--->|  arg[0]/ret[0]  | <----+ <-------- arg0ret0AddrReg
               |    sliceSize    |
               |   frame_size    |
               +-----------------+
                  (low address)
```

Finally, the trampoline jumps into the execution of the host function using the
`exitSequence` meta-instruction.

Upon return, the process is reversed.

## Code

- The trampoline to enter the generated function is implemented by the
  `backend.Machine.CompileEntryPreamble()` method.
- The trampoline to return traps and invoke host functions is generated by
  `backend.Machine.CompileGoFunctionTrampoline()` method.

You can find arch-specific implementations in
`backend/isa/<arch>/abi_go_call.go`,
`backend/isa/<arch>/abi_entry_preamble.go`, etc. The trampolines are found
under `backend/isa/<arch>/abi_entry_<arch>.s`.

## Further References

- Go's [internal ABI documentation][abi-internal] details the calling convention similar to the one we use in both arm64 and amd64 backend.
- Raphael Poss's [The Go low-level calling convention on
  x86-64][go-call-conv-x86] is also an excellent reference for `amd64`.

[abi-internal]: https://tip.golang.org/src/cmd/compile/abi-internal
[go-call-conv-x86]: https://dr-knz.net/go-calling-convention-x86-64.html
[proposal-register-cc]: https://go.googlesource.com/proposal/+/master/design/40724-register-calling.md#background
[how-do-compiler-functions-work]: ../../how_do_compiler_functions_work/

