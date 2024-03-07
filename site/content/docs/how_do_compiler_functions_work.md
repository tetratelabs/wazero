# How do compiler functions work?

WebAssembly runtimes let you call functions defined in wasm. How this works in
wazero is different depending on your `RuntimeConfig`.

* `RuntimeConfigCompiler` compiles machine code from your wasm, and jumps to
  that when invoking a function.
* `RuntimeConfigInterpreter` does not generate code. It interprets wasm and
  executes go statements that correspond to WebAssembly instructions.

How the compiler works precisely is a large topic, and discussed at length on
this page. For more general information on architecture, etc., please refer to
[Docs](..).

## Engines

Our [Docs](..) introduce the "engine" concept of wazero. More precisely, there
are three types of engines, `Engine`, `ModuleEngine` and `callEngine`. Each has
a different scope and role:

- `Engine` has the same lifetime as `Runtime`. This compiles a `CompiledModule`
  into machine code, which is both cached and memory-mapped as an executable.
- `ModuleEngine` is a virtual machine with the same lifetime as its [Module][api-module].
  Notably, this binds each [function instance][spec-function-instance] to
  corresponding machine code owned by its `Engine`.
- `callEngine` is the implementation of [api.Function][api-function] in a
  [Module][api-module]. This implements `Function.Call(...)` by invoking
  machine code corresponding to a function instance in `ModuleEngine` and
  managing the [call stack][call-stack] representing the invocation.

Here is a diagram showing the relationships of these engines:

```goat
      .-----------> Instantiated module                                 Exported Function
     /1:N                   |                                                  |
    /                       |                                                  v
   |     +----------+       v        +----------------+                  +------------+
   |     |  Engine  |--------------->|  ModuleEngine  |----------------->| callEngine |
   |     +----------+                +----------------+                  +------------+
   |          |                               |                            |      |
   .          |                               |                            |      |
 main.wasm -->|        .--------------------->|          '-----------------+      |
              |       /                       |          |                        |
              v      .                        v          v                        v
      +--------------+      +-----------------------------------+            +----------+
      | Machine Code |      |[(func_instance, machine_code),...]|            |Call Stack|
      +--------------+      +-----------------------------------+            +----------+
                                               ^                                  ^
                                               |                                  |
                                               |                                  |
                                               +----------------------------------+
                                                               |
                                                               |
                                                               |
                                                        Function.Call()
```

## Callbacks from machine code to Go

Go source can be compiled to invoke native library functions using CGO.
However, [CGO is not GO][cgo-not-go]. To call native functions in pure Go, we
need a different approach with unique constraints.

The most notable constraints are:
* machine code must not manipulate the Goroutine or system stack
* we cannot modify the signal handler of Go at runtime

### Handling the call stack

One constraint is the generated machine code must not manipulate Goroutine
(or system) stack. Otherwise, the Go runtime gets corrupted, which results in
fatal execution errors. This means we cannot[^1] call Go functions (host
functions) directly from machine code (compiled from wasm). This is routinely
needed in WebAssembly, as system calls such as WASI are defined in Go, but
invoked from Wasm. To handle this, we employ a "trampoline strategy".

Let's explain the "trampoline strategy" with an example. `random_get` is a host
function defined in Go, called from machine code compiled from guest `main`
function. Let's say the wasm function corresponding to that is called `_start`.
`_start` function is called by wazero by default on `Instantiate`.

Here is a TinyGo source file describing this.
```go
//go:import wasi_snapshot_preview1 random_get
func random_get(age int32)package main

import "unsafe"

// random_get is a function defined on the host, specifically, the wazero
// program written in Go.
//
//go:wasmimport wasi_snapshot_preview1 random_get
func random_get(ptr uintptr, size uint32) (errno uint32)

// main is compiled to wasm, so this is the guest. Conventionally, this ends up
// named `_start`.
func main() {
    // Define a buffer to hold random data
	size := uint32(8)
    buf := make([]byte, size)

	// Fill the buffer with random data using an imported host function.
    // The host needs to know where in guest memory to place the random data.
	// To communicate this, we have to convert buf to a uintptr.
    errno := random_get(uintptr(unsafe.Pointer(&buf[0])), size)
    if errno != 0 {
        panic(errno)
    }
}
```

When `_start` calls `random_get`, it exits execution first. wazero calls the Go
function mapped to `random_get` like a usual Go program. Finally, wazero
transfers control back to machine code again, resuming `_start` after the call
instruction to `random_get`.

Here's what the "trampoline strategy" looks like in a diagram. For simplicity,
we'll say the wasm memory offset of the `buf` is zero, but it will be different
in real execution.
```goat
   |                                     Go              |           Machine Code
   |                                                           (compiled from main.wasm)
   |                                                     |
   v
   |                        `Instantiate(ctx, mainWasm)` |
   |                                     |
   v                                     v               |
   |                            +----------------+                  +------------+
   |                            |func exec_native|-------|--------> |func _start |
   v                            +----------------+                  +------------+
   |                                                     |         /
   |            Go func call    +----------------+                / ptr=0,size=8
   v           .----------------|func exec_native|<------|-------. status=call_host_fn(name=rand_get)
   |          /  ptr=0,size=8   +----------------+     exit
   |         v                                           |
   v   +-------------+          +----------------+
   |   |func rand_get|--------->|func exec_native|-------|-------.
   |   +-------------+ errno=0  +----------------+    continue    \ errno=0
   v                                                     |         \
   |                                                     |          +------------+
   |                                                     |          |func _start |
   v                                                     |          +------------+
```

### Signal handling

Code compiled to wasm use [runtime traps][spec-trap] to abort execution. For
example, a `panic` compiled with TinyGo becomes a wasm function named
`runtime._panic`, which issues an [unreachable][spec-unreachable] instruction
after printing the message to STDERR.

```go
package main

func main() {
	panic("help")
}
```

Native JIT compilers set custom signal handlers for [Wasm runtime traps][spec-trap],
such as the [unreachable][spec-unreachable] instruction. However, we cannot
safely [modify the signal handler of Go at runtime][signal-handler-discussion].
As described in the first section, wazero always exits the execution of machine
code. Machine code sets status when it encounters an `unreachable` instruction.
This is read by wazero, which propagates it back with `ErrRuntimeUnreachable`.

Here's a diagram showing this:
```goat
   |                               Go                 |                             Machine Code
   |                                                                          (compiled from main.wasm)
   |                                                  |
   v
   |                   `Instantiate(ctx, mainWasm)`   |
   |                                |
   v                                v                 |
   |                       +----------------+                                     +------------+
   |                       |func exec_native|---------|-------------------------> |func _start |
   v                       +----------------+                                     +------------+
   |                                                  |                                 |
   |                       +----------------+                  exit           +--------------------+
   v                       |func exec_native|<--------|---------------------- |func runtime._panic |
   |                       +----------------+            status=unreachable   +--------------------+
   |                              |                   |
   v                              |
   |                panic(WasmRuntimeErrUnreachable)  |
```

One thing you will notice above is that the calls between wasm functions, such
as from `_start` to `runtime._panic` do not use a trampoline. The trampoline
strategy is only used between wasm and the host.

## Summary

When an exported wasm function is called, using a wazero API, such as
`Function.Call()`, wazero allocates a `callEngine` and starts invocation. This
begins with jumping to machine code compiled from the Wasm binary. When that
code makes a callback to the host, it exits execution, passing control back to
`exec_native` which then calls a Go function and resumes the machine code
afterwards. In the face of Wasm runtime errors, we exit the machine code
execution with the proper status, and return the control back to `exec_native`
function, just like host function calls. Just instead of calling a Go function,
we call `panic` with a corresponding error. This jumping is why the strategy is
called a trampoline, and only used between the guest wasm and the host running
it.

[call-stack]: https://en.wikipedia.org/wiki/Call_stack
[api-function]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Function
[api-module]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Module
[spec-function-instance]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0
[spec-trap]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#trap
[spec-unreachable]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-instr-control
[signal-handler-discussion]: https://gophers.slack.com/archives/C1C1YSQBT/p1675992411241409
[cgo-not-go]: https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s

[^1]: it's technically possible to call it directly, but that would come with performing "stack switching" in the native code.
  It's almost the same as what wazero does: exiting the execution of machine code, then call the target Go function (using the caller of machine code as a "trampoline").
